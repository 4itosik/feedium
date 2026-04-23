package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"

	"github.com/4itosik/feedium/internal/biz"
	entgo "github.com/4itosik/feedium/internal/ent"
)

type txKey struct{}

// txState holds the transactional context shared by Ent and raw SQL repos.
// The *ent.Client is backed by the same [sql.Tx], so both surfaces commit/rollback
// atomically.
type txState struct {
	client *entgo.Client
	sqlTx  *sql.Tx
}

// sqlExecer is the subset of [sql.DB] / [sql.Tx] we use from raw-SQL repos.
type sqlExecer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func clientFromContext(ctx context.Context, fallback *entgo.Client) *entgo.Client {
	if state, ok := ctx.Value(txKey{}).(*txState); ok {
		return state.client
	}
	return fallback
}

func sqlExecerFromContext(ctx context.Context, fallback *sql.DB) sqlExecer {
	if state, ok := ctx.Value(txKey{}).(*txState); ok {
		return state.sqlTx
	}
	return fallback
}

type txManager struct {
	data *Data
}

var _ biz.TxManager = (*txManager)(nil)

func NewTxManager(data *Data) *txManager { //nolint:revive // unexported return type for wire DI
	return &txManager{data: data}
}

func (m *txManager) InTx(ctx context.Context, fn func(ctx context.Context) error) error {
	sqlTx, beginErr := m.data.DB.BeginTx(ctx, nil)
	if beginErr != nil {
		return fmt.Errorf("begin tx: %w", beginErr)
	}

	drv := entsql.NewDriver(dialect.Postgres, entsql.Conn{ExecQuerier: sqlTx})
	entClient := entgo.NewClient(entgo.Driver(drv))

	state := &txState{client: entClient, sqlTx: sqlTx}
	txCtx := context.WithValue(ctx, txKey{}, state)

	fnErr := fn(txCtx)
	if fnErr != nil {
		if rbErr := sqlTx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			return fmt.Errorf("fn error: %w, rollback error: %w", fnErr, rbErr)
		}
		return fnErr
	}

	if commitErr := sqlTx.Commit(); commitErr != nil {
		return fmt.Errorf("commit tx: %w", commitErr)
	}
	return nil
}
