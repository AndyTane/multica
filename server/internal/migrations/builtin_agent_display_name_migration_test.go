package migrations

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const builtinAgentDisplayNameTestSchema = "builtin_agent_display_name_test"

// TestBuiltinAgentDisplayNameMigration covers CLAWSTA-85 batch A: the built-in
// Chief of Staff is stored with system_key 'mika' but displayed as Siman. The
// migration must rename only the display name the product shipped, keep every
// other agent alone, and stay safe when the runner executes it twice (a failed
// ledger write replays the file on the next startup).
func TestBuiltinAgentDisplayNameMigration(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("integration test requires Postgres at DATABASE_URL")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect to Postgres: %v", err)
	}
	defer pool.Close()

	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire Postgres connection: %v", err)
	}
	defer conn.Release()

	cleanup := func() {
		_, _ = pool.Exec(context.Background(), "DROP SCHEMA IF EXISTS "+builtinAgentDisplayNameTestSchema+" CASCADE")
	}
	cleanup()
	t.Cleanup(cleanup)
	if _, err := conn.Exec(ctx, "CREATE SCHEMA "+builtinAgentDisplayNameTestSchema); err != nil {
		t.Fatalf("create isolated migration schema: %v", err)
	}
	if _, err := conn.Exec(ctx, `SELECT set_config('search_path', $1, false)`, builtinAgentDisplayNameTestSchema); err != nil {
		t.Fatalf("set isolated migration search path: %v", err)
	}
	if _, err := conn.Exec(ctx, `
		CREATE TABLE agent (
			id UUID PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			system_key TEXT,
			updated_at TIMESTAMPTZ NOT NULL
		)
	`); err != nil {
		t.Fatalf("create pre-migration agent table: %v", err)
	}

	const enSeed = "Your workspace Chief of Staff. Mika turns goals into issues, coordinates agents, and helps build reusable workflows."
	const zhSeed = "你的工作区 Chief of Staff。Mika 会把目标转化为任务、协调智能体，并帮你建立可复用的工作流。"
	const koSeed = "워크스페이스의 Chief of Staff입니다. Mika가 목표를 태스크로 구체화하고 에이전트를 조율하며 재사용 가능한 워크플로 구성을 돕습니다."
	const enAfter = "Your workspace Chief of Staff. Siman turns goals into issues, coordinates agents, and helps build reusable workflows."
	const zhAfter = "你的工作区 Chief of Staff。Siman 会把目标转化为任务、协调智能体，并帮你建立可复用的工作流。"
	const koAfter = "워크스페이스의 Chief of Staff입니다. Siman이 목표를 태스크로 구체화하고 에이전트를 조율하며 재사용 가능한 워크플로 구성을 돕습니다."

	seeded := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := conn.Exec(ctx, `
		INSERT INTO agent (id, name, description, system_key, updated_at) VALUES
			('58300000-0000-4000-8000-000000000001', 'Mika', $1, 'mika', $4),
			('58300000-0000-4000-8000-000000000002', 'Jarvis', $2, 'mika', $4),
			('58300000-0000-4000-8000-000000000003', 'Mika', 'My friend Mika helps me', NULL, $4),
			('58300000-0000-4000-8000-000000000004', 'Mika', 'Our own chief of staff', 'mika', $4),
			('58300000-0000-4000-8000-000000000005', 'Mika', $3, 'some_other_key', $4)
	`, enSeed, zhSeed, koSeed, seeded); err != nil {
		t.Fatalf("seed pre-migration agents: %v", err)
	}

	// The runner records each version after executing its SQL; a ledger write
	// failure replays the same file on the next startup, so apply it twice.
	applyMigrationFile(t, ctx, conn.Conn(), "583_builtin_agent_display_name.up.sql")
	applyMigrationFile(t, ctx, conn.Conn(), "583_builtin_agent_display_name.up.sql")

	type row struct {
		name, description string
		updatedAt         time.Time
	}
	read := func(id string) row {
		t.Helper()
		var r row
		if err := conn.QueryRow(ctx,
			`SELECT name, description, updated_at FROM agent WHERE id = $1`, id).Scan(&r.name, &r.description, &r.updatedAt); err != nil {
			t.Fatalf("read agent %s: %v", id, err)
		}
		return r
	}

	renamed := read("58300000-0000-4000-8000-000000000001")
	if renamed.name != "Siman" || renamed.description != enAfter {
		t.Fatalf("built-in agent = %q / %q, want Siman with the migrated English description", renamed.name, renamed.description)
	}
	if !renamed.updatedAt.After(seeded) {
		t.Fatalf("changed agent updated_at = %s, want it bumped past %s", renamed.updatedAt, seeded)
	}

	ownerRenamed := read("58300000-0000-4000-8000-000000000002")
	if ownerRenamed.name != "Jarvis" {
		t.Fatalf("owner-chosen display name = %q, want it preserved", ownerRenamed.name)
	}
	if ownerRenamed.description != zhAfter {
		t.Fatalf("seeded Chinese description = %q, want the migrated copy", ownerRenamed.description)
	}

	ordinary := read("58300000-0000-4000-8000-000000000003")
	if ordinary.name != "Mika" || ordinary.description != "My friend Mika helps me" || !ordinary.updatedAt.Equal(seeded) {
		t.Fatalf("ordinary agent was rewritten: %+v", ordinary)
	}

	ownerDescription := read("58300000-0000-4000-8000-000000000004")
	if ownerDescription.name != "Siman" || ownerDescription.description != "Our own chief of staff" {
		t.Fatalf("owner-edited description = %q / %q, want the name migrated and the description kept", ownerDescription.name, ownerDescription.description)
	}

	otherSystemKey := read("58300000-0000-4000-8000-000000000005")
	if otherSystemKey.name != "Mika" || otherSystemKey.description != koSeed || !otherSystemKey.updatedAt.Equal(seeded) {
		t.Fatalf("agent with another system key was rewritten: %+v", otherSystemKey)
	}

	applyMigrationFile(t, ctx, conn.Conn(), "583_builtin_agent_display_name.down.sql")

	reverted := read("58300000-0000-4000-8000-000000000001")
	if reverted.name != "Mika" || reverted.description != enSeed {
		t.Fatalf("down migration = %q / %q, want the pre-migration name and description", reverted.name, reverted.description)
	}
	if got := read("58300000-0000-4000-8000-000000000002"); got.name != "Jarvis" || got.description != zhSeed {
		t.Fatalf("down migration rewrote an owner-chosen name: %+v", got)
	}
	if got := read("58300000-0000-4000-8000-000000000004"); got.description != "Our own chief of staff" {
		t.Fatalf("down migration rewrote an owner-edited description: %+v", got)
	}
	if got := read("58300000-0000-4000-8000-000000000005"); got.name != "Mika" || got.description != koSeed || !got.updatedAt.Equal(seeded) {
		t.Fatalf("down migration touched an agent it never migrated: %+v", got)
	}
}
