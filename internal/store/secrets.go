package store

import (
	"context"
	"errors"
	"strconv"

	"gorm.io/gorm"

	"model-check/internal/secretbox"
)

// ErrNoSecretKey means saved keys cannot be stored or used: the server was
// started without a master key.
var ErrNoSecretKey = errors.New("saved-key encryption is not configured")

// secretAAD binds a ciphertext to its row: a value copied into another
// credential or account does not decrypt.
func secretAAD(id string, userID int64) string {
	return id + "|" + strconv.FormatInt(userID, 10)
}

// UseSecretBox sets the master key and encrypts any saved key still stored
// in plaintext (rows from before encryption). It is idempotent.
// A key that cannot open the rows already encrypted is refused.
func (s *Store) UseSecretBox(ctx context.Context, box *secretbox.Box) (int, error) {
	n, err := s.reseal(ctx, nil, box)
	if err == nil {
		s.box = box
	}
	return n, err
}

// RotateSecretKey re-encrypts every saved key from the current master key to
// next, in one transaction, and switches to next.
func (s *Store) RotateSecretKey(ctx context.Context, next *secretbox.Box) (int, error) {
	if s.box == nil {
		return 0, ErrNoSecretKey
	}
	n, err := s.reseal(ctx, s.box, next)
	if err == nil {
		s.box = next
	}
	return n, err
}

// reseal rewrites rows with to: plaintext rows always, and sealed rows when
// from is set (rotation). A sealed row that from cannot open fails the whole
// transaction, so a wrong key never leaves a mix behind.
func (s *Store) reseal(ctx context.Context, from, to *secretbox.Box) (int, error) {
	changed := 0
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		rows := []Credential{}
		if err := tx.Select("id", "user_id", "secret").Find(&rows).Error; err != nil {
			return err
		}
		for _, r := range rows {
			aad := secretAAD(r.ID, r.UserID)
			plain := r.Secret
			if secretbox.Sealed(r.Secret) {
				if from == nil {
					// Already encrypted: it must open with the configured key.
					if _, err := to.Open(r.Secret, aad); err != nil {
						return err
					}
					continue
				}
				var err error
				if plain, err = from.Open(r.Secret, aad); err != nil {
					return err
				}
			}
			sealed, err := to.Seal(plain, aad)
			if err != nil {
				return err
			}
			if err := tx.Model(&Credential{}).Where("id = ?", r.ID).Update("secret", sealed).Error; err != nil {
				return err
			}
			changed++
		}
		return nil
	})
	return changed, err
}
