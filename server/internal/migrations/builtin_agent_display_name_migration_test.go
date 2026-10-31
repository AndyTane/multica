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
// migration must rename only the display name the product shipped, translate
// every localized description the product seeded — including Korean, whose
// particle after the name changes with it — and leave every other agent alone.
// The runner can replay a file whose ledger write failed, so running it twice
// must not touch a row or bump updated_at. The down direction must restore
// exactly the product's own copy and nothing an owner edited, in both
// directions and per language.
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

	// The exact copy the server wrote before and after the rename. The mapping
	// has to move between these strings: replacing "Mika" with "Siman" inside
	// the Korean sentence would produce "Siman가", while the server now writes
	// "Siman이", so the seeded description has to be mapped per language.
	const (
		enSeed  = "Your workspace Chief of Staff. Mika turns goals into issues, coordinates agents, and helps build reusable workflows."
		zhSeed  = "你的工作区 Chief of Staff。Mika 会把目标转化为任务、协调智能体，并帮你建立可复用的工作流。"
		koSeed  = "워크스페이스의 Chief of Staff입니다. Mika가 목표를 태스크로 구체화하고 에이전트를 조율하며 재사용 가능한 워크플로 구성을 돕습니다."
		jaSeed  = "ワークスペースの Chief of Staff。Mika は目標をタスクに落とし込み、エージェントを調整し、再利用できるワークフローづくりを支援します。"
		enAfter = "Your workspace Chief of Staff. Siman turns goals into issues, coordinates agents, and helps build reusable workflows."
		zhAfter = "你的工作区 Chief of Staff。Siman 会把目标转化为任务、协调智能体，并帮你建立可复用的工作流。"
		koAfter = "워크스페이스의 Chief of Staff입니다. Siman이 목표를 태스크로 구체화하고 에이전트를 조율하며 재사용 가능한 워크플로 구성을 돕습니다."
		jaAfter = "ワークスペースの Chief of Staff。Siman は目標をタスクに落とし込み、エージェントを調整し、再利用できるワークフローづくりを支援します。"
	)

	const (
		enBuiltinID  = "58300000-0000-4000-8000-000000000001"
		zhRenamedID  = "58300000-0000-4000-8000-000000000002"
		ordinaryID   = "58300000-0000-4000-8000-000000000003"
		customDescID = "58300000-0000-4000-8000-000000000004"
		otherKeyID   = "58300000-0000-4000-8000-000000000005"
		koBuiltinID  = "58300000-0000-4000-8000-000000000006"
		jaBuiltinID  = "58300000-0000-4000-8000-000000000007"
		postUpID     = "58300000-0000-4000-8000-000000000008"
	)
	ids := []string{enBuiltinID, zhRenamedID, ordinaryID, customDescID, otherKeyID, koBuiltinID, jaBuiltinID, postUpID}

	seeded := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := conn.Exec(ctx, `
		INSERT INTO agent (id, name, description, system_key, updated_at) VALUES
			('58300000-0000-4000-8000-000000000001', 'Mika', $1, 'mika', $5),
			('58300000-0000-4000-8000-000000000002', 'Jarvis', $2, 'mika', $5),
			('58300000-0000-4000-8000-000000000003', 'Mika', 'My friend Mika helps me', NULL, $5),
			('58300000-0000-4000-8000-000000000004', 'Mika', 'Our own chief of staff', 'mika', $5),
			('58300000-0000-4000-8000-000000000005', 'Mika', $3, 'some_other_key', $5),
			('58300000-0000-4000-8000-000000000006', 'Mika', $3, 'mika', $5),
			('58300000-0000-4000-8000-000000000007', 'Mika', $4, 'mika', $5),
			('58300000-0000-4000-8000-000000000008', 'Siman', $6, 'mika', $5)
	`, enSeed, zhSeed, koSeed, jaSeed, seeded, koAfter); err != nil {
		t.Fatalf("seed pre-migration agents: %v", err)
	}

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
	snapshot := func() map[string]row {
		t.Helper()
		out := make(map[string]row, len(ids))
		for _, id := range ids {
			out[id] = read(id)
		}
		return out
	}
	assertUnchanged := func(when string, want map[string]row) {
		t.Helper()
		for _, id := range ids {
			got, expected := read(id), want[id]
			if got.name != expected.name || got.description != expected.description || !got.updatedAt.Equal(expected.updatedAt) {
				t.Fatalf("%s rewrote agent %s: %+v, want %+v", when, id, got, expected)
			}
		}
	}

	// The runner records each version after executing its SQL; a ledger write
	// failure replays the same file on the next startup.
	applyMigrationFile(t, ctx, conn.Conn(), "583_builtin_agent_display_name.up.sql")

	renamed := read(enBuiltinID)
	if renamed.name != "Siman" || renamed.description != enAfter {
		t.Fatalf("built-in agent = %q / %q, want Siman with the migrated English description", renamed.name, renamed.description)
	}
	if !renamed.updatedAt.After(seeded) {
		t.Fatalf("changed agent updated_at = %s, want it bumped past %s", renamed.updatedAt, seeded)
	}

	ownerRenamed := read(zhRenamedID)
	if ownerRenamed.name != "Jarvis" {
		t.Fatalf("owner-chosen display name = %q, want it preserved", ownerRenamed.name)
	}
	if ownerRenamed.description != zhAfter {
		t.Fatalf("seeded Chinese description = %q, want the migrated copy", ownerRenamed.description)
	}

	// The Korean row is the one a name-only REPLACE gets wrong: Mika가 has to
	// become Siman이, not Siman가.
	korean := read(koBuiltinID)
	if korean.name != "Siman" || korean.description != koAfter {
		t.Fatalf("Korean built-in agent = %q / %q, want Siman with the server's new Korean copy", korean.name, korean.description)
	}

	japanese := read(jaBuiltinID)
	if japanese.name != "Siman" || japanese.description != jaAfter {
		t.Fatalf("Japanese built-in agent = %q / %q, want Siman with the migrated Japanese description", japanese.name, japanese.description)
	}

	if got := read(ordinaryID); got.name != "Mika" || got.description != "My friend Mika helps me" || !got.updatedAt.Equal(seeded) {
		t.Fatalf("ordinary agent was rewritten: %+v", got)
	}
	if got := read(customDescID); got.name != "Siman" || got.description != "Our own chief of staff" {
		t.Fatalf("owner-edited description = %q / %q, want the name migrated and the description kept", got.name, got.description)
	}
	if got := read(otherKeyID); got.name != "Mika" || got.description != koSeed || !got.updatedAt.Equal(seeded) {
		t.Fatalf("agent with another system key was rewritten: %+v", got)
	}
	// A row the server created after the rename already carries the new copy;
	// up must not touch it, and down must still translate it back when rolling
	// back, because the server will write the old copy again.
	if got := read(postUpID); got.name != "Siman" || got.description != koAfter || !got.updatedAt.Equal(seeded) {
		t.Fatalf("agent created after the rename was rewritten by up: %+v", got)
	}

	afterUp := snapshot()
	applyMigrationFile(t, ctx, conn.Conn(), "583_builtin_agent_display_name.up.sql")
	assertUnchanged("replaying the up migration", afterUp)

	applyMigrationFile(t, ctx, conn.Conn(), "583_builtin_agent_display_name.down.sql")

	reverted := read(enBuiltinID)
	if reverted.name != "Mika" || reverted.description != enSeed {
		t.Fatalf("down migration = %q / %q, want the pre-migration name and description", reverted.name, reverted.description)
	}
	if got := read(koBuiltinID); got.name != "Mika" || got.description != koSeed {
		t.Fatalf("down migration = %q / %q, want the Korean copy the product originally shipped", got.name, got.description)
	}
	if got := read(jaBuiltinID); got.name != "Mika" || got.description != jaSeed {
		t.Fatalf("down migration = %q / %q, want the Japanese copy the product originally shipped", got.name, got.description)
	}
	if got := read(zhRenamedID); got.name != "Jarvis" || got.description != zhSeed {
		t.Fatalf("down migration rewrote an owner-chosen name: %+v", got)
	}
	if got := read(customDescID); got.name != "Mika" || got.description != "Our own chief of staff" {
		t.Fatalf("down migration rewrote an owner-edited description: %+v", got)
	}
	if got := read(ordinaryID); got.name != "Mika" || got.description != "My friend Mika helps me" || !got.updatedAt.Equal(seeded) {
		t.Fatalf("down migration touched an agent it never migrated: %+v", got)
	}
	if got := read(otherKeyID); got.name != "Mika" || got.description != koSeed || !got.updatedAt.Equal(seeded) {
		t.Fatalf("down migration touched an agent with another system key: %+v", got)
	}
	if got := read(postUpID); got.name != "Mika" || got.description != koSeed {
		t.Fatalf("down migration on a row created after the rename = %q / %q, want the original name and Korean copy", got.name, got.description)
	}

	afterDown := snapshot()
	applyMigrationFile(t, ctx, conn.Conn(), "583_builtin_agent_display_name.down.sql")
	assertUnchanged("replaying the down migration", afterDown)
}
