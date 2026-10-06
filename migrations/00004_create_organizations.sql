-- +goose Up
CREATE TABLE biznes.organizations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT organizations_name_valid CHECK (
        char_length(name) BETWEEN 1 AND 120 AND name !~ '[[:cntrl:]]'
    )
);

CREATE TABLE biznes.memberships (
    organization_id uuid NOT NULL REFERENCES biznes.organizations(id) ON DELETE RESTRICT,
    user_id uuid NOT NULL REFERENCES biznes.users(id) ON DELETE RESTRICT,
    role text COLLATE "C" NOT NULL,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (organization_id, user_id),
    CONSTRAINT memberships_role_valid CHECK (role IN ('owner', 'admin', 'accountant', 'staff'))
);

CREATE INDEX memberships_user_organization_idx ON biznes.memberships (user_id, organization_id);

-- +goose Down
LOCK TABLE biznes.organizations, biznes.memberships IN ACCESS EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM biznes.organizations) OR EXISTS (SELECT 1 FROM biznes.memberships) THEN
        RAISE EXCEPTION 'organizations migration rollback requires empty tables';
    END IF;
END;
$$;
-- +goose StatementEnd
DROP TABLE biznes.memberships RESTRICT;
DROP TABLE biznes.organizations RESTRICT;
