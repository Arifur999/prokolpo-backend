-- +goose Up
-- The dashboard card (design v2, board Dashboard): "প্রজেক্ট N" numbers an owner's projects in the
-- order they were opened, and "জমা হয়েছে" is the money that came into the project.
ALTER TABLE projects ADD COLUMN project_no integer;

UPDATE projects p SET project_no = n.no
FROM (SELECT id, row_number() OVER (PARTITION BY owner_id ORDER BY created_at, id) AS no FROM projects) n
WHERE n.id = p.id;

ALTER TABLE projects ALTER COLUMN project_no SET NOT NULL;
ALTER TABLE projects ADD CONSTRAINT projects_owner_no_key UNIQUE (owner_id, project_no);

-- A new project takes the owner's next number. The owner's row is locked so two projects opened
-- at once can't take the same one; a deleted project keeps its number.
-- +goose StatementBegin
CREATE FUNCTION number_project() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  PERFORM 1 FROM users WHERE id = NEW.owner_id FOR UPDATE;
  SELECT coalesce(max(project_no), 0) + 1 INTO NEW.project_no FROM projects WHERE owner_id = NEW.owner_id;
  RETURN NEW;
END $$;
-- +goose StatementEnd

CREATE TRIGGER projects_number BEFORE INSERT ON projects FOR EACH ROW EXECUTE FUNCTION number_project();

-- Money in: opening balances, partner deposits, installment collections, farm receipts, the
-- shop's takings and credit paid back. Transfers between the project's own accounts are not
-- new money and stay out. Mirrors the inflow branches of the accounts ledger (internal/books).
-- +goose StatementBegin
CREATE FUNCTION project_deposited(p uuid) RETURNS bigint LANGUAGE sql STABLE AS $$
  SELECT coalesce((SELECT sum(opening_balance) FROM money_accounts WHERE project_id = p AND opening_balance > 0), 0)::bigint
       + coalesce((SELECT sum(amount) FROM partner_deposits WHERE project_id = p), 0)::bigint
       + coalesce((SELECT sum(amount) FROM sale_payments WHERE project_id = p), 0)::bigint
       + coalesce((SELECT sum(amount) FROM produce_receipts WHERE project_id = p), 0)::bigint
       + coalesce((SELECT sum(cash + other + mobile) FROM shop_days WHERE project_id = p), 0)::bigint
       + coalesce((SELECT sum(amount) FROM khata_entries WHERE project_id = p AND kind = 'PAYMENT'), 0)::bigint
$$;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER projects_number ON projects;
DROP FUNCTION number_project();
DROP FUNCTION project_deposited(uuid);
ALTER TABLE projects DROP CONSTRAINT projects_owner_no_key;
ALTER TABLE projects DROP COLUMN project_no;
