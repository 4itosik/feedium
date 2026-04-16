package data

import (
	"context"
	"fmt"

	entgo "github.com/4itosik/feedium/internal/ent"

	"github.com/4itosik/feedium/internal/biz"
)

type txKey struct{}

func clientFromContext(ctx context.Context, fallback *entgo.Client) *entgo.Client {
	if tx, ok := ctx.Value(txKey{}).(*entgo.Tx); ok {
		return tx.Client()
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
	tx, txErr := m.data.Ent.Tx(ctx)
	if txErr != nil {
		return fmt.Errorf("begin tx: %w", txErr)
	}

	txCtx := context.WithValue(ctx, txKey{}, tx)

	fnErr := fn(txCtx)
	if fnErr != nil {
		rollbackErr := tx.Rollback()
		if rollbackErr != nil {
			return fmt.Errorf("fn error: %w, rollback error: %w", fnErr, rollbackErr)
		}
		return fnErr
	}

	commitErr := tx.Commit()
	if commitErr != nil {
		return fmt.Errorf("commit tx: %w", commitErr)
	}
	return nil
}
