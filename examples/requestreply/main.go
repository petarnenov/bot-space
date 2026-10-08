// requestreply runs one requester or responder process using the official MCP SDK.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"syscall"
	"time"

	"github.com/petarnenov/bot-space/internal/agents"
	"github.com/petarnenov/bot-space/internal/mailbox"
	"github.com/petarnenov/bot-space/internal/mcpclient"
	"github.com/petarnenov/bot-space/internal/security"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Request/reply demo failed:", err)
		os.Exit(1)
	}
}

func run() error {
	role := flag.String("role", "", "requester or responder")
	demo := flag.String("demo-id", os.Getenv("DEMO_ID"), "shared unique run ID (1–40 ASCII label characters)")
	timeout := flag.Duration("timeout", 45*time.Second, "maximum wait, at most five minutes")
	flag.Parse()
	if (*role != "requester" && *role != "responder") || !regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,40}$`).MatchString(*demo) || *timeout <= 0 || *timeout > 5*time.Minute {
		return errors.New("configure valid role, shared demo ID, and bounded timeout")
	}
	peer := os.Getenv("PEER_AGENT_ID")
	if !security.ValidUUID(peer) {
		return errors.New("PEER_AGENT_ID must identify the other agent")
	}
	endpoint := os.Getenv("MCP_URL")
	if endpoint == "" {
		endpoint = "http://localhost:8080/mcp"
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	client, err := mcpclient.Connect(ctx, endpoint, os.Getenv("MCP_AGENT_TOKEN"))
	if err != nil {
		return err
	}
	defer client.Close()
	var who agents.Principal
	if err = client.Call(ctx, "whoami", map[string]any{}, &who); err != nil {
		return err
	}
	if who.AgentID == peer {
		return errors.New("requester and responder must have different agent identities")
	}
	requestKind, replyKind := *demo+".request", *demo+".reply"
	if *role == "requester" {
		var sent mailbox.Message
		err = client.Call(ctx, "send_message", map[string]any{"to_agent_id": peer, "idempotency_key": "demo:" + *demo + ":request", "kind": requestKind, "text": "Synthetic request/reply demonstration", "metadata": map[string]any{"demo_id": *demo}}, &sent)
		if err != nil {
			return err
		}
		fmt.Println("Requester sent message", sent.ID)
		for {
			var page mailbox.Page
			if err = client.Call(ctx, "read_messages", map[string]any{"thread_id": sent.ThreadID, "kind": replyKind, "acknowledged": "all"}, &page); err != nil {
				return err
			}
			for _, reply := range page.Messages {
				if reply.FromAgentID == peer && reply.InReplyTo != nil && *reply.InReplyTo == sent.ID {
					var ack mailbox.Message
					if err = client.Call(ctx, "acknowledge_message", map[string]any{"message_id": reply.ID}, &ack); err != nil {
						return err
					}
					fmt.Println("Requester read and acknowledged reply", reply.ID)
					return nil
				}
			}
			if err = pollWait(ctx); err != nil {
				return err
			}
		}
	}
	for {
		var page mailbox.Page
		if err = client.Call(ctx, "read_messages", map[string]any{"kind": requestKind}, &page); err != nil {
			return err
		}
		for _, request := range page.Messages {
			if request.FromAgentID == peer {
				// Processing here only constructs a synthetic response. Business workers
				// need their own durable/idempotent processing; this is no execution lease.
				var ack mailbox.Message
				if err = client.Call(ctx, "acknowledge_message", map[string]any{"message_id": request.ID}, &ack); err != nil {
					return err
				}
				var reply mailbox.Message
				if err = client.Call(ctx, "send_message", map[string]any{"to_agent_id": peer, "idempotency_key": "reply:" + request.ID, "kind": replyKind, "text": "Synthetic response", "metadata": map[string]any{"demo_id": *demo}, "in_reply_to": request.ID}, &reply); err != nil {
					return err
				}
				fmt.Println("Responder read, acknowledged, and replied to", request.ID, "with", reply.ID)
				return nil
			}
		}
		if err = pollWait(ctx); err != nil {
			return err
		}
	}
}

func pollWait(ctx context.Context) error {
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return errors.New("demo wait deadline reached")
	case <-timer.C:
		return nil
	}
}
