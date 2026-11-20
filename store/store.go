package store

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const encryptedFilePrefix = "QEMUGLAZE_ENC_V1:"

// Options configures the local persistent key-value store.
type Options struct {
	Path          string
	EncryptionKey string
	MemoryOnly    bool
}

// Store persists JSON values atomically on disk with optional AES-256-GCM encryption.
type Store struct {
	mu         sync.RWMutex
	path       string
	aead       cipher.AEAD
	memoryOnly bool
	data       map[string]jsontext.Value
}

// New initializes a persistent store under os.UserConfigDir()/<appID>/store.json.
func New(appID string, opts Options) (*Store, error) {
	if appID == "" {
		appID = "qemu-glaze"
	}

	s := &Store{
		memoryOnly: opts.MemoryOnly,
		data:       make(map[string]jsontext.Value),
	}

	if opts.EncryptionKey != "" {
		hash := sha256.Sum256([]byte(opts.EncryptionKey))
		block, err := aes.NewCipher(hash[:])
		if err != nil {
			return nil, fmt.Errorf("store cipher init: %w", err)
		}
		aead, err := cipher.NewGCM(block)
		if err != nil {
			return nil, fmt.Errorf("store gcm init: %w", err)
		}
		s.aead = aead
	}

	if !opts.MemoryOnly {
		storePath := opts.Path
		if storePath == "" {
			cfgDir, err := os.UserConfigDir()
			if err != nil || cfgDir == "" {
				cfgDir = os.TempDir()
			}
			storePath = filepath.Join(cfgDir, appID, "store.json")
		}
		s.path = storePath
		if err := s.load(); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}

	return s, nil
}

// Path returns the filesystem path backing this store.
func (s *Store) Path() string {
	return s.path
}

// Set encodes value as JSON, stores it under key, and persists atomically to disk.
func (s *Store) Set(key string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("store marshal %q: %w", key, err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.data[key] = jsontext.Value(raw)
	return s.saveLocked()
}

// Get decodes the JSON value stored at key into dest, returning false when absent.
func (s *Store) Get(key string, dest any) (bool, error) {
	s.mu.RLock()
	raw, ok := s.data[key]
	s.mu.RUnlock()

	if !ok {
		return false, nil
	}
	if dest == nil {
		return true, nil
	}
	if err := json.Unmarshal(raw, dest); err != nil {
		return true, fmt.Errorf("store unmarshal %q: %w", key, err)
	}
	return true, nil
}

func (s *Store) load() error {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return nil
	}

	var plain []byte
	if payload, encrypted := strings.CutPrefix(trimmed, encryptedFilePrefix); encrypted {
		if s.aead == nil {
			return errors.New("store is encrypted but no encryption key was provided")
		}
		cipherBytes, err := base64.StdEncoding.DecodeString(payload)
		if err != nil {
			return fmt.Errorf("store decode ciphertext: %w", err)
		}
		nonceSize := s.aead.NonceSize()
		if len(cipherBytes) < nonceSize {
			return errors.New("store ciphertext too short")
		}
		nonce, ct := cipherBytes[:nonceSize], cipherBytes[nonceSize:]
		plain, err = s.aead.Open(nil, nonce, ct, nil)
		if err != nil {
			return fmt.Errorf("store decrypt failed: %w", err)
		}
	} else {
		plain = raw
	}

	var loaded map[string]jsontext.Value
	if err := json.Unmarshal(plain, &loaded); err != nil {
		return fmt.Errorf("store parse json: %w", err)
	}
	if loaded != nil {
		s.data = loaded
	}
	return nil
}

func (s *Store) saveLocked() error {
	if s.memoryOnly || s.path == "" {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return fmt.Errorf("store mkdir: %w", err)
	}

	plain, err := json.Marshal(s.data, jsontext.WithIndent("  "))
	if err != nil {
		return fmt.Errorf("store encode json: %w", err)
	}

	outBytes := plain
	if s.aead != nil {
		nonce := make([]byte, s.aead.NonceSize())
		if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
			return fmt.Errorf("store nonce: %w", err)
		}
		sealed := s.aead.Seal(nonce, nonce, plain, nil)
		outBytes = []byte(encryptedFilePrefix + base64.StdEncoding.EncodeToString(sealed))
	}

	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, outBytes, 0600); err != nil {
		return fmt.Errorf("store write temp: %w", err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("store rename: %w", err)
	}
	return nil
}
