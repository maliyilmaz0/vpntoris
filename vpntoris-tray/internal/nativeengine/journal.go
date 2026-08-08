package nativeengine

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"sync"
	"time"
)

var safeIdentifier = regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,96}$`)

type Journal struct {
	directory string
	owner     string
	mu        sync.Mutex
}

func NewJournal(directory, owner string) (*Journal, error) {
	if directory == "" || !safeIdentifier.MatchString(owner) {
		return nil, fmt.Errorf("invalid journal configuration")
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	if err := os.Chmod(directory, 0700); err != nil {
		return nil, err
	}
	return &Journal{directory: directory, owner: owner}, nil
}
func (journal *Journal) Begin(profile string) (*Transaction, error) {
	if profile == "" || len(profile) > 160 {
		return nil, fmt.Errorf("invalid profile")
	}
	id, err := randomIdentifier()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	transaction := &Transaction{Version: 1, ID: id, Owner: journal.owner, Profile: profile, Platform: runtime.GOOS, State: TransactionPreparing, CreatedAt: now, UpdatedAt: now, Mutations: []Mutation{}}
	if err := journal.Save(transaction); err != nil {
		return nil, err
	}
	return transaction, nil
}
func (journal *Journal) Save(transaction *Transaction) error {
	journal.mu.Lock()
	defer journal.mu.Unlock()
	return journal.saveLocked(transaction)
}
func (journal *Journal) saveLocked(transaction *Transaction) error {
	if err := journal.validate(transaction); err != nil {
		return err
	}
	transaction.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(transaction, "", "  ")
	if err != nil {
		return err
	}
	path := journal.path(transaction.ID)
	temporary, err := os.CreateTemp(journal.directory, ".transaction-")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	directory, err := os.Open(journal.directory)
	if err == nil {
		err = directory.Sync()
		directory.Close()
	}
	return err
}
func (journal *Journal) Load(id string) (*Transaction, error) {
	journal.mu.Lock()
	defer journal.mu.Unlock()
	if !safeIdentifier.MatchString(id) {
		return nil, fmt.Errorf("invalid transaction id")
	}
	return journal.loadLocked(journal.path(id))
}
func (journal *Journal) List() ([]*Transaction, error) {
	journal.mu.Lock()
	defer journal.mu.Unlock()
	entries, err := os.ReadDir(journal.directory)
	if err != nil {
		return nil, err
	}
	transactions := []*Transaction{}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		transaction, loadError := journal.loadLocked(filepath.Join(journal.directory, entry.Name()))
		if loadError != nil {
			return nil, loadError
		}
		transactions = append(transactions, transaction)
	}
	sort.Slice(transactions, func(i, k int) bool { return transactions[i].CreatedAt.Before(transactions[k].CreatedAt) })
	return transactions, nil
}
func (journal *Journal) Remove(id string) error {
	journal.mu.Lock()
	defer journal.mu.Unlock()
	if !safeIdentifier.MatchString(id) {
		return fmt.Errorf("invalid transaction id")
	}
	err := os.Remove(journal.path(id))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
func (journal *Journal) loadLocked(path string) (*Transaction, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var transaction Transaction
	if err := json.Unmarshal(data, &transaction); err != nil {
		return nil, err
	}
	if err := journal.validate(&transaction); err != nil {
		return nil, err
	}
	return &transaction, nil
}
func (journal *Journal) validate(transaction *Transaction) error {
	if transaction == nil || transaction.Version != 1 || !safeIdentifier.MatchString(transaction.ID) || transaction.Owner != journal.owner || transaction.Profile == "" || transaction.Platform == "" {
		return fmt.Errorf("invalid transaction")
	}
	return nil
}
func (journal *Journal) path(id string) string {
	return filepath.Join(journal.directory, id+".json")
}
func randomIdentifier() (string, error) {
	data := make([]byte, 16)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return hex.EncodeToString(data), nil
}
