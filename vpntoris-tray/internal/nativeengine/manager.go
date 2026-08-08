package nativeengine

import (
	"context"
	"fmt"
)

type Backend interface {
	Apply(context.Context, Mutation) error
	Undo(context.Context, Mutation) error
	Owned(context.Context, Mutation) (bool, error)
}
type Manager struct {
	journal *Journal
	backend Backend
}

func NewManager(journal *Journal, backend Backend) (*Manager, error) {
	if journal == nil || backend == nil {
		return nil, fmt.Errorf("journal and backend are required")
	}
	return &Manager{journal: journal, backend: backend}, nil
}
func (manager *Manager) Activate(ctx context.Context, plan Plan) (*Transaction, error) {
	transaction, err := manager.journal.Begin(plan.Profile)
	if err != nil {
		return nil, err
	}
	for index := range plan.Mutations {
		mutation := plan.Mutations[index]
		if mutation.ID == "" {
			mutation.ID, err = randomIdentifier()
			if err != nil {
				return transaction, manager.rollback(ctx, transaction, err)
			}
		}
		mutation.State = MutationPending
		transaction.Mutations = append(transaction.Mutations, mutation)
		if err := manager.journal.Save(transaction); err != nil {
			return transaction, manager.rollback(ctx, transaction, err)
		}
		if err := manager.backend.Apply(ctx, mutation); err != nil {
			transaction.Mutations[len(transaction.Mutations)-1].State = MutationFailed
			transaction.Mutations[len(transaction.Mutations)-1].Error = err.Error()
			_ = manager.journal.Save(transaction)
			return transaction, manager.rollback(ctx, transaction, err)
		}
		transaction.Mutations[len(transaction.Mutations)-1].State = MutationApplied
		if err := manager.journal.Save(transaction); err != nil {
			return transaction, manager.rollback(ctx, transaction, err)
		}
	}
	transaction.State = TransactionActive
	if err := manager.journal.Save(transaction); err != nil {
		return transaction, manager.rollback(ctx, transaction, err)
	}
	return transaction, nil
}
func (manager *Manager) Deactivate(ctx context.Context, transaction *Transaction) error {
	if transaction == nil {
		return fmt.Errorf("transaction is required")
	}
	return manager.rollback(ctx, transaction, nil)
}
func (manager *Manager) Recover(ctx context.Context) error {
	transactions, err := manager.journal.List()
	if err != nil {
		return err
	}
	var recoveryError error
	for _, transaction := range transactions {
		if err := manager.rollback(ctx, transaction, nil); err != nil {
			recoveryError = fmt.Errorf("recover %s: %w", transaction.Profile, err)
		}
	}
	return recoveryError
}
func (manager *Manager) rollback(ctx context.Context, transaction *Transaction, cause error) error {
	transaction.State = TransactionRollingBack
	_ = manager.journal.Save(transaction)
	var rollbackError error
	for index := len(transaction.Mutations) - 1; index >= 0; index-- {
		mutation := &transaction.Mutations[index]
		if mutation.State != MutationPending && mutation.State != MutationApplied && mutation.State != MutationFailed {
			continue
		}
		owned, err := manager.backend.Owned(ctx, *mutation)
		if err != nil {
			rollbackError = err
			continue
		}
		if owned {
			if err := manager.backend.Undo(ctx, *mutation); err != nil {
				mutation.Error = err.Error()
				rollbackError = err
				continue
			}
		}
		mutation.State = MutationUndone
		mutation.Error = ""
		_ = manager.journal.Save(transaction)
	}
	if rollbackError == nil {
		if err := manager.journal.Remove(transaction.ID); err != nil {
			rollbackError = err
		}
	} else {
		transaction.State = TransactionFailed
		_ = manager.journal.Save(transaction)
	}
	if cause != nil && rollbackError != nil {
		return fmt.Errorf("%v; rollback failed: %w", cause, rollbackError)
	}
	if cause != nil {
		return cause
	}
	return rollbackError
}
