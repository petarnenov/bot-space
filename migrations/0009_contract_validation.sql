CREATE TABLE mailbox.contract_validations (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 project_id uuid NOT NULL,
 contract_id uuid NOT NULL,
 contract_hash text NOT NULL CHECK(contract_hash ~ '^[0-9a-f]{64}$'),
 validator_runner_id uuid NOT NULL,
 validated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(project_id,contract_id,contract_hash),
 FOREIGN KEY(project_id,contract_id) REFERENCES mailbox.work_contracts(project_id,id),
 FOREIGN KEY(project_id,validator_runner_id) REFERENCES mailbox.project_runners(project_id,id)
);
