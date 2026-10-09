ALTER TABLE mailbox.council_decisions ADD COLUMN subject text NOT NULL DEFAULT 'root'
 CHECK(octet_length(subject) BETWEEN 1 AND 256);
ALTER TABLE mailbox.council_decisions ADD CONSTRAINT council_plan_root_subject
 CHECK(kind!='plan' OR subject='root');
ALTER TABLE mailbox.council_decisions ADD UNIQUE(project_id,id,root_id,kind,subject);
ALTER TABLE mailbox.council_decisions ADD FOREIGN KEY(project_id,previous_id,root_id,kind,subject)
 REFERENCES mailbox.council_decisions(project_id,id,root_id,kind,subject);
ALTER TABLE mailbox.council_heads ADD COLUMN subject text NOT NULL DEFAULT 'root'
 CHECK(octet_length(subject) BETWEEN 1 AND 256);
ALTER TABLE mailbox.council_heads DROP CONSTRAINT council_heads_pkey;
ALTER TABLE mailbox.council_heads ADD PRIMARY KEY(project_id,root_id,kind,subject);
ALTER TABLE mailbox.council_heads ADD FOREIGN KEY(project_id,decision_id,root_id,kind,subject)
 REFERENCES mailbox.council_decisions(project_id,id,root_id,kind,subject);
