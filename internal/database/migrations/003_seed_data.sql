-- +goose Up
-- Seed workspace for development
INSERT INTO workspaces (id, name, slug, status_deadlines, created_at)
VALUES
    ('00000000-0000-0000-0000-000000000001',
     'MTL Agents',
     'mtl-agents',
     '{"NEW": 120, "IN_PROGRESS": 1440, "BLOCKED": 2880}'::jsonb,
     NOW());

-- Seed agents for development
INSERT INTO agents (id, workspace_id, name, token, is_active, created_at)
VALUES
    ('00000000-0000-0000-0000-000000000101',
     '00000000-0000-0000-0000-000000000001',
     'bot-alpha',
     'slp_dev_alpha_' || replace(uuid_generate_v4()::text, '-', ''),
     true,
     NOW()),
    ('00000000-0000-0000-0000-000000000102',
     '00000000-0000-0000-0000-000000000001',
     'bot-beta',
     'slp_dev_beta_' || replace(uuid_generate_v4()::text, '-', ''),
     true,
     NOW()),
    ('00000000-0000-0000-0000-000000000103',
     '00000000-0000-0000-0000-000000000001',
     'bot-gamma',
     'slp_dev_gamma_' || replace(uuid_generate_v4()::text, '-', ''),
     true,
     NOW());

-- +goose Down
-- Agents are automatically deleted via CASCADE
DELETE FROM workspaces WHERE id = '00000000-0000-0000-0000-000000000001';
