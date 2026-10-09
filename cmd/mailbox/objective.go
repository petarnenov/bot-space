package main

import (
	"errors"
	"flag"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/petarnenov/bot-space/internal/backlog"
	"github.com/petarnenov/bot-space/internal/githubapp"
	"github.com/petarnenov/bot-space/internal/repositoryaccess"
	"github.com/petarnenov/bot-space/internal/security"
)

type objectiveOptions struct {
	GitHubID int64
	Input    backlog.Input
}

func parseObjectiveOptions(args []string, getenv func(string) string, readFile func(string) (string, error)) (objectiveOptions, error) {
	flags := flag.NewFlagSet("objective", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	project := flags.String("project", getenv("OBJECTIVE_PROJECT"), "Project UUID")
	githubDefault, _ := strconv.ParseInt(getenv("OBJECTIVE_GITHUB_USER_ID"), 10, 64)
	githubID := flags.Int64("github-user-id", githubDefault, "Immutable GitHub user ID")
	title := flags.String("title", getenv("OBJECTIVE_TITLE"), "Objective title")
	description := flags.String("description", getenv("OBJECTIVE_DESCRIPTION"), "Objective description")
	descriptionFile := flags.String("description-file", getenv("OBJECTIVE_DESCRIPTION_FILE"), "Objective description file")
	priorityDefault := 2
	if raw := getenv("OBJECTIVE_PRIORITY"); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil {
			priorityDefault = value
		} else {
			priorityDefault = -1
		}
	}
	priority := flags.Int("priority", priorityDefault, "Priority from 0 to 4")
	ticket := flags.String("ticket", getenv("OBJECTIVE_TICKET"), "Ticket reference")
	key := flags.String("key", getenv("OBJECTIVE_KEY"), "Idempotency key")
	if flags.Parse(args) != nil || flags.NArg() != 0 || *project == "" || *githubID < 1 || strings.TrimSpace(*title) == "" || (*description == "") == (*descriptionFile == "") || *priority < 0 || *priority > 4 {
		return objectiveOptions{}, errors.New("invalid objective arguments")
	}
	if *descriptionFile != "" {
		var err error
		*description, err = readFile(*descriptionFile)
		if err != nil {
			return objectiveOptions{}, errors.New("objective description file could not be read")
		}
	}
	if strings.TrimSpace(*description) == "" {
		return objectiveOptions{}, errors.New("invalid objective arguments")
	}
	if *key == "" {
		var err error
		*key, err = security.Secret()
		if err != nil {
			return objectiveOptions{}, errors.New("objective key could not be generated")
		}
	}
	return objectiveOptions{GitHubID: *githubID, Input: backlog.Input{ProjectID: *project, Key: *key, Title: *title, Description: *description, Ticket: *ticket, Priority: *priority}}, nil
}

func readDescriptionFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, 32769))
	if err != nil || len(raw) > 32768 {
		return "", errors.New("objective description file is too large")
	}
	return string(raw), nil
}

func operatorAuthority(getenv func(string) string) (*repositoryaccess.Checker, error) {
	installation, err := strconv.ParseInt(getenv("GITHUB_APP_INSTALLATION_ID"), 10, 64)
	if err != nil {
		return nil, errors.New("GitHub App verification configuration required")
	}
	app, err := githubapp.New(getenv("GITHUB_APP_CLIENT_ID"), installation, []byte(getenv("GITHUB_APP_PRIVATE_KEY")))
	if err != nil {
		return nil, errors.New("GitHub App verification configuration required")
	}
	checker, err := repositoryaccess.New(app.Token)
	if err != nil {
		return nil, errors.New("GitHub App verification configuration required")
	}
	return checker, nil
}

func objectiveError(err error) error {
	switch {
	case errors.Is(err, backlog.ErrInvalid):
		return errors.New("objective input is invalid")
	case errors.Is(err, backlog.ErrForbidden):
		return errors.New("objective creator or project access is invalid")
	case errors.Is(err, backlog.ErrConflict):
		return errors.New("objective idempotency key conflicts with another payload")
	default:
		return errors.New("objective could not be created")
	}
}
