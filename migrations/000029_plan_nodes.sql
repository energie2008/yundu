-- +goose Up
-- +goose StatementBegin

CREATE TABLE IF NOT EXISTS plan_nodes (
  plan_id UUID NOT NULL REFERENCES plans(id) ON DELETE CASCADE,
  node_id UUID NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (plan_id, node_id)
);
CREATE INDEX IF NOT EXISTS idx_plan_nodes_plan_id ON plan_nodes(plan_id);
CREATE INDEX IF NOT EXISTS idx_plan_nodes_node_id ON plan_nodes(node_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS plan_nodes;
-- +goose StatementEnd
