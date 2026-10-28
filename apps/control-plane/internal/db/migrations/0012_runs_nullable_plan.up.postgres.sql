-- Runs outlive their plan: plan_id becomes nullable so ON DELETE SET NULL works.
ALTER TABLE runs ALTER COLUMN plan_id DROP NOT NULL;
