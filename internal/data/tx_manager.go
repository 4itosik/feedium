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

func NewTxManager(data *Data) *txManager {
	return &txManager{data: data}
}

func (m *txManager) InTx(ctx context.Context, fn func(ctx context.Context) error) error {
	tx, err := m.data.Ent.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	txCtx := context.WithValue(ctx, txKey{}, tx)

	if err := fn(txCtx); err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return fmt.Errorf("fn error: %w, rollback error: %v", err, rollbackErr)
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}
