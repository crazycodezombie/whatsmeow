// Copyright (c) 2022 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package sqlstore contains an SQL-backed implementation of the interfaces in the store package.
package sqlstore

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/util/keys"
)

// ErrInvalidLength is returned by some database getters if the database returned a byte array with an unexpected length.
// This should be impossible, as the database schema contains CHECK()s for all the relevant columns.
var ErrInvalidLength = errors.New("database returned byte array with illegal length")

// PostgresArrayWrapper is a function to wrap array values before passing them to the sql package.
//
// When using github.com/lib/pq, you should set
//
//	whatsmeow.PostgresArrayWrapper = pq.Array
var PostgresArrayWrapper func(interface{}) interface {
	driver.Valuer
	sql.Scanner
}

type SQLStore struct {
	*Container
	JID string

	preKeyLock sync.Mutex

	contactCache     map[types.JID]*types.ContactInfo
	contactCacheLock sync.Mutex

	archivedCache     map[types.JID]*store.ArchivedEntry
	archivedCacheLock sync.RWMutex

	labelsCache     map[int]types.LabelInfo
	labelsCacheLock sync.Mutex
}

// NewSQLStore creates a new SQLStore with the given database container and user JID.
// It contains implementations of all the different stores in the store package.
//
// In general, you should use Container.NewDevice or Container.GetDevice instead of this.
func NewSQLStore(c *Container, jid types.JID) *SQLStore {
	return &SQLStore{
		Container:     c,
		JID:           jid.String(),
		contactCache:  make(map[types.JID]*types.ContactInfo),
		archivedCache: make(map[types.JID]*store.ArchivedEntry),
		labelsCache:   make(map[int]types.LabelInfo),
	}
}

var _ store.IdentityStore = (*SQLStore)(nil)
var _ store.SessionStore = (*SQLStore)(nil)
var _ store.PreKeyStore = (*SQLStore)(nil)
var _ store.SenderKeyStore = (*SQLStore)(nil)
var _ store.AppStateSyncKeyStore = (*SQLStore)(nil)
var _ store.AppStateStore = (*SQLStore)(nil)
var _ store.ContactStore = (*SQLStore)(nil)
var _ store.LabelsStore = (*SQLStore)(nil)

const (
	putIdentityQuery = `
		INSERT INTO whatsmeow_identity_keys (our_jid, their_id, identity) VALUES ($1, $2, $3)
		ON CONFLICT (our_jid, their_id) DO UPDATE SET identity=excluded.identity
	`
	deleteAllIdentitiesQuery = `DELETE FROM whatsmeow_identity_keys WHERE our_jid=$1 AND their_id LIKE $2`
	deleteIdentityQuery      = `DELETE FROM whatsmeow_identity_keys WHERE our_jid=$1 AND their_id=$2`
	getIdentityQuery         = `SELECT identity FROM whatsmeow_identity_keys WHERE our_jid=$1 AND their_id=$2`
)

func (s *SQLStore) PutIdentity(address string, key [32]byte) error {
	_, err := s.db.Exec(putIdentityQuery, s.JID, address, key[:])
	return err
}

func (s *SQLStore) DeleteAllIdentities(phone string) error {
	_, err := s.db.Exec(deleteAllIdentitiesQuery, s.JID, phone+":%")
	return err
}

func (s *SQLStore) DeleteIdentity(address string) error {
	_, err := s.db.Exec(deleteAllIdentitiesQuery, s.JID, address)
	return err
}

func (s *SQLStore) IsTrustedIdentity(address string, key [32]byte) (bool, error) {
	var existingIdentity []byte
	err := s.db.QueryRow(getIdentityQuery, s.JID, address).Scan(&existingIdentity)
	if errors.Is(err, sql.ErrNoRows) {
		// Trust if not known, it'll be saved automatically later
		return true, nil
	} else if err != nil {
		return false, err
	} else if len(existingIdentity) != 32 {
		return false, ErrInvalidLength
	}
	return *(*[32]byte)(existingIdentity) == key, nil
}

const (
	getSessionQuery = `SELECT session FROM whatsmeow_sessions WHERE our_jid=$1 AND their_id=$2`
	hasSessionQuery = `SELECT true FROM whatsmeow_sessions WHERE our_jid=$1 AND their_id=$2`
	putSessionQuery = `
		INSERT INTO whatsmeow_sessions (our_jid, their_id, session) VALUES ($1, $2, $3)
		ON CONFLICT (our_jid, their_id) DO UPDATE SET session=excluded.session
	`
	deleteAllSessionsQuery = `DELETE FROM whatsmeow_sessions WHERE our_jid=$1 AND their_id LIKE $2`
	deleteSessionQuery     = `DELETE FROM whatsmeow_sessions WHERE our_jid=$1 AND their_id=$2`
)

func (s *SQLStore) GetSession(address string) (session []byte, err error) {
	err = s.db.QueryRow(getSessionQuery, s.JID, address).Scan(&session)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	return
}

func (s *SQLStore) HasSession(address string) (has bool, err error) {
	err = s.db.QueryRow(hasSessionQuery, s.JID, address).Scan(&has)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	return
}

func (s *SQLStore) PutSession(address string, session []byte) error {
	_, err := s.db.Exec(putSessionQuery, s.JID, address, session)
	return err
}

func (s *SQLStore) DeleteAllSessions(phone string) error {
	_, err := s.db.Exec(deleteAllSessionsQuery, s.JID, phone+":%")
	return err
}

func (s *SQLStore) DeleteSession(address string) error {
	_, err := s.db.Exec(deleteSessionQuery, s.JID, address)
	return err
}

const (
	getLastPreKeyIDQuery        = `SELECT MAX(key_id) FROM whatsmeow_pre_keys WHERE jid=$1`
	insertPreKeyQuery           = `INSERT INTO whatsmeow_pre_keys (jid, key_id, key, uploaded) VALUES ($1, $2, $3, $4)`
	getUnuploadedPreKeysQuery   = `SELECT key_id, key FROM whatsmeow_pre_keys WHERE jid=$1 AND uploaded=false ORDER BY key_id LIMIT $2`
	getPreKeyQuery              = `SELECT key_id, key FROM whatsmeow_pre_keys WHERE jid=$1 AND key_id=$2`
	deletePreKeyQuery           = `DELETE FROM whatsmeow_pre_keys WHERE jid=$1 AND key_id=$2`
	markPreKeysAsUploadedQuery  = `UPDATE whatsmeow_pre_keys SET uploaded=true WHERE jid=$1 AND key_id<=$2`
	getUploadedPreKeyCountQuery = `SELECT COUNT(*) FROM whatsmeow_pre_keys WHERE jid=$1 AND uploaded=true`
)

func (s *SQLStore) genOnePreKey(id uint32, markUploaded bool) (*keys.PreKey, error) {
	key := keys.NewPreKey(id)
	_, err := s.db.Exec(insertPreKeyQuery, s.JID, key.KeyID, key.Priv[:], markUploaded)
	return key, err
}

func (s *SQLStore) getNextPreKeyID() (uint32, error) {
	var lastKeyID sql.NullInt32
	err := s.db.QueryRow(getLastPreKeyIDQuery, s.JID).Scan(&lastKeyID)
	if err != nil {
		return 0, fmt.Errorf("failed to query next prekey ID: %w", err)
	}
	return uint32(lastKeyID.Int32) + 1, nil
}

func (s *SQLStore) GenOnePreKey() (*keys.PreKey, error) {
	s.preKeyLock.Lock()
	defer s.preKeyLock.Unlock()
	nextKeyID, err := s.getNextPreKeyID()
	if err != nil {
		return nil, err
	}
	return s.genOnePreKey(nextKeyID, true)
}

func (s *SQLStore) GetOrGenPreKeys(count uint32) ([]*keys.PreKey, error) {
	s.preKeyLock.Lock()
	defer s.preKeyLock.Unlock()

	res, err := s.db.Query(getUnuploadedPreKeysQuery, s.JID, count)
	if err != nil {
		return nil, fmt.Errorf("failed to query existing prekeys: %w", err)
	}
	newKeys := make([]*keys.PreKey, count)
	var existingCount uint32
	for res.Next() {
		var key *keys.PreKey
		key, err = scanPreKey(res)
		if err != nil {
			return nil, err
		} else if key != nil {
			newKeys[existingCount] = key
			existingCount++
		}
	}

	if existingCount < uint32(len(newKeys)) {
		var nextKeyID uint32
		nextKeyID, err = s.getNextPreKeyID()
		if err != nil {
			return nil, err
		}
		for i := existingCount; i < count; i++ {
			newKeys[i], err = s.genOnePreKey(nextKeyID, false)
			if err != nil {
				return nil, fmt.Errorf("failed to generate prekey: %w", err)
			}
			nextKeyID++
		}
	}

	return newKeys, nil
}

func scanPreKey(row scannable) (*keys.PreKey, error) {
	var priv []byte
	var id uint32
	err := row.Scan(&id, &priv)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, err
	} else if len(priv) != 32 {
		return nil, ErrInvalidLength
	}
	return &keys.PreKey{
		KeyPair: *keys.NewKeyPairFromPrivateKey(*(*[32]byte)(priv)),
		KeyID:   id,
	}, nil
}

func (s *SQLStore) GetPreKey(id uint32) (*keys.PreKey, error) {
	return scanPreKey(s.db.QueryRow(getPreKeyQuery, s.JID, id))
}

func (s *SQLStore) RemovePreKey(id uint32) error {
	_, err := s.db.Exec(deletePreKeyQuery, s.JID, id)
	return err
}

func (s *SQLStore) MarkPreKeysAsUploaded(upToID uint32) error {
	_, err := s.db.Exec(markPreKeysAsUploadedQuery, s.JID, upToID)
	return err
}

func (s *SQLStore) UploadedPreKeyCount() (count int, err error) {
	err = s.db.QueryRow(getUploadedPreKeyCountQuery, s.JID).Scan(&count)
	return
}

const (
	getSenderKeyQuery = `SELECT sender_key FROM whatsmeow_sender_keys WHERE our_jid=$1 AND chat_id=$2 AND sender_id=$3`
	putSenderKeyQuery = `
		INSERT INTO whatsmeow_sender_keys (our_jid, chat_id, sender_id, sender_key) VALUES ($1, $2, $3, $4)
		ON CONFLICT (our_jid, chat_id, sender_id) DO UPDATE SET sender_key=excluded.sender_key
	`
)

func (s *SQLStore) PutSenderKey(group, user string, session []byte) error {
	_, err := s.db.Exec(putSenderKeyQuery, s.JID, group, user, session)
	return err
}

func (s *SQLStore) GetSenderKey(group, user string) (key []byte, err error) {
	err = s.db.QueryRow(getSenderKeyQuery, s.JID, group, user).Scan(&key)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	return
}

const (
	putAppStateSyncKeyQuery = `
		INSERT INTO whatsmeow_app_state_sync_keys (jid, key_id, key_data, timestamp, fingerprint) VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (jid, key_id) DO UPDATE
			SET key_data=excluded.key_data, timestamp=excluded.timestamp, fingerprint=excluded.fingerprint
			WHERE excluded.timestamp > whatsmeow_app_state_sync_keys.timestamp
	`
	getAppStateSyncKeyQuery         = `SELECT key_data, timestamp, fingerprint FROM whatsmeow_app_state_sync_keys WHERE jid=$1 AND key_id=$2`
	getLatestAppStateSyncKeyIDQuery = `SELECT key_id FROM whatsmeow_app_state_sync_keys WHERE jid=$1 ORDER BY timestamp DESC LIMIT 1`
)

func (s *SQLStore) PutAppStateSyncKey(id []byte, key store.AppStateSyncKey) error {
	_, err := s.db.Exec(putAppStateSyncKeyQuery, s.JID, id, key.Data, key.Timestamp, key.Fingerprint)
	return err
}

func (s *SQLStore) GetAppStateSyncKey(id []byte) (*store.AppStateSyncKey, error) {
	var key store.AppStateSyncKey
	err := s.db.QueryRow(getAppStateSyncKeyQuery, s.JID, id).Scan(&key.Data, &key.Timestamp, &key.Fingerprint)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &key, err
}

func (s *SQLStore) GetLatestAppStateSyncKeyID() ([]byte, error) {
	var keyID []byte
	err := s.db.QueryRow(getLatestAppStateSyncKeyIDQuery, s.JID).Scan(&keyID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return keyID, err
}

const (
	putAppStateVersionQuery = `
		INSERT INTO whatsmeow_app_state_version (jid, name, version, hash) VALUES ($1, $2, $3, $4)
		ON CONFLICT (jid, name) DO UPDATE SET version=excluded.version, hash=excluded.hash
	`
	getAppStateVersionQuery                 = `SELECT version, hash FROM whatsmeow_app_state_version WHERE jid=$1 AND name=$2`
	deleteAppStateVersionQuery              = `DELETE FROM whatsmeow_app_state_version WHERE jid=$1 AND name=$2`
	putAppStateMutationMACsQuery            = `INSERT INTO whatsmeow_app_state_mutation_macs (jid, name, version, index_mac, value_mac) VALUES `
	deleteAppStateMutationMACsQueryPostgres = `DELETE FROM whatsmeow_app_state_mutation_macs WHERE jid=$1 AND name=$2 AND index_mac=ANY($3::bytea[])`
	deleteAppStateMutationMACsQueryGeneric  = `DELETE FROM whatsmeow_app_state_mutation_macs WHERE jid=$1 AND name=$2 AND index_mac IN `
	getAppStateMutationMACQuery             = `SELECT value_mac FROM whatsmeow_app_state_mutation_macs WHERE jid=$1 AND name=$2 AND index_mac=$3 ORDER BY version DESC LIMIT 1`
)

func (s *SQLStore) PutAppStateVersion(name string, version uint64, hash [128]byte) error {
	_, err := s.db.Exec(putAppStateVersionQuery, s.JID, name, version, hash[:])
	return err
}

func (s *SQLStore) GetAppStateVersion(name string) (version uint64, hash [128]byte, err error) {
	var uncheckedHash []byte
	err = s.db.QueryRow(getAppStateVersionQuery, s.JID, name).Scan(&version, &uncheckedHash)
	if errors.Is(err, sql.ErrNoRows) {
		// version will be 0 and hash will be an empty array, which is the correct initial state
		err = nil
	} else if err != nil {
		// There's an error, just return it
	} else if len(uncheckedHash) != 128 {
		// This shouldn't happen
		err = ErrInvalidLength
	} else {
		// No errors, convert hash slice to array
		hash = *(*[128]byte)(uncheckedHash)
	}
	return
}

func (s *SQLStore) DeleteAppStateVersion(name string) error {
	_, err := s.db.Exec(deleteAppStateVersionQuery, s.JID, name)
	return err
}

type execable interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
}

func (s *SQLStore) putAppStateMutationMACs(tx execable, name string, version uint64, mutations []store.AppStateMutationMAC) error {
	values := make([]interface{}, 3+len(mutations)*2)
	queryParts := make([]string, len(mutations))
	values[0] = s.JID
	values[1] = name
	values[2] = version
	placeholderSyntax := "($1, $2, $3, $%d, $%d)"
	if s.dialect == "sqlite3" {
		placeholderSyntax = "(?1, ?2, ?3, ?%d, ?%d)"
	}
	for i, mutation := range mutations {
		baseIndex := 3 + i*2
		values[baseIndex] = mutation.IndexMAC
		values[baseIndex+1] = mutation.ValueMAC
		queryParts[i] = fmt.Sprintf(placeholderSyntax, baseIndex+1, baseIndex+2)
	}
	_, err := tx.Exec(putAppStateMutationMACsQuery+strings.Join(queryParts, ","), values...)
	return err
}

const mutationBatchSize = 400

func (s *SQLStore) PutAppStateMutationMACs(name string, version uint64, mutations []store.AppStateMutationMAC) error {
	if len(mutations) > mutationBatchSize {
		tx, err := s.db.Begin()
		if err != nil {
			return fmt.Errorf("failed to start transaction: %w", err)
		}
		for i := 0; i < len(mutations); i += mutationBatchSize {
			var mutationSlice []store.AppStateMutationMAC
			if len(mutations) > i+mutationBatchSize {
				mutationSlice = mutations[i : i+mutationBatchSize]
			} else {
				mutationSlice = mutations[i:]
			}
			err = s.putAppStateMutationMACs(tx, name, version, mutationSlice)
			if err != nil {
				_ = tx.Rollback()
				return err
			}
		}
		err = tx.Commit()
		if err != nil {
			return fmt.Errorf("failed to commit transaction: %w", err)
		}
		return nil
	} else if len(mutations) > 0 {
		return s.putAppStateMutationMACs(s.db, name, version, mutations)
	}
	return nil
}

func (s *SQLStore) DeleteAppStateMutationMACs(name string, indexMACs [][]byte) (err error) {
	if len(indexMACs) == 0 {
		return
	}
	if s.dialect == "postgres" && PostgresArrayWrapper != nil {
		_, err = s.db.Exec(deleteAppStateMutationMACsQueryPostgres, s.JID, name, PostgresArrayWrapper(indexMACs))
	} else {
		args := make([]interface{}, 2+len(indexMACs))
		args[0] = s.JID
		args[1] = name
		queryParts := make([]string, len(indexMACs))
		for i, item := range indexMACs {
			args[2+i] = item
			queryParts[i] = fmt.Sprintf("$%d", i+3)
		}
		_, err = s.db.Exec(deleteAppStateMutationMACsQueryGeneric+"("+strings.Join(queryParts, ",")+")", args...)
	}
	return
}

func (s *SQLStore) GetAppStateMutationMAC(name string, indexMAC []byte) (valueMAC []byte, err error) {
	err = s.db.QueryRow(getAppStateMutationMACQuery, s.JID, name, indexMAC).Scan(&valueMAC)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	return
}

const (
	putContactNameQuery = `
		INSERT INTO whatsmeow_contacts (our_jid, their_jid, first_name, full_name, is_phone_contact) VALUES ($1, $2, $3, $4, TRUE)
		ON CONFLICT (our_jid, their_jid) DO UPDATE SET first_name=excluded.first_name, full_name=excluded.full_name
	`
	deleteContactNameQuery      = `DELETE FROM whatsmeow_contacts WHERE our_jid=$1 AND their_jid=$2`
	deleteManyContactNamesQuery = `DELETE FROM whatsmeow_contacts WHERE our_jid=$1 AND their_jid IN (%s)`
	putManyContactNamesQuery    = `
		INSERT INTO whatsmeow_contacts (our_jid, their_jid, first_name, full_name, is_phone_contact)
		VALUES %s
		ON CONFLICT (our_jid, their_jid) DO UPDATE SET first_name=excluded.first_name, full_name=excluded.full_name
	`
	putPushNameQuery = `
		INSERT INTO whatsmeow_contacts (our_jid, their_jid, push_name, is_phone_contact) VALUES ($1, $2, $3, FALSE)
		ON CONFLICT (our_jid, their_jid) DO UPDATE SET push_name=excluded.push_name
	`
	putBusinessNameQuery = `
		INSERT INTO whatsmeow_contacts (our_jid, their_jid, business_name, is_phone_contact) VALUES ($1, $2, $3, FALSE)
		ON CONFLICT (our_jid, their_jid) DO UPDATE SET business_name=excluded.business_name
	`
	getContactQuery = `
		SELECT first_name, full_name, push_name, business_name, is_phone_contact FROM whatsmeow_contacts WHERE our_jid=$1 AND their_jid=$2
	`
	getAllContactsQuery = `
		SELECT their_jid, first_name, full_name, push_name, business_name, is_phone_contact FROM whatsmeow_contacts WHERE our_jid=$1
	`
)

func (s *SQLStore) PutPushName(user types.JID, pushName string) (bool, string, error) {
	s.contactCacheLock.Lock()
	defer s.contactCacheLock.Unlock()

	cached, err := s.getContact(user)
	if err != nil {
		return false, "", err
	}
	if cached.PushName != pushName {
		_, err = s.db.Exec(putPushNameQuery, s.JID, user, pushName)
		if err != nil {
			return false, "", err
		}
		previousName := cached.PushName
		cached.PushName = pushName
		cached.Found = true
		return true, previousName, nil
	}
	return false, "", nil
}

func (s *SQLStore) PutBusinessName(user types.JID, businessName string) (bool, string, error) {
	s.contactCacheLock.Lock()
	defer s.contactCacheLock.Unlock()

	cached, err := s.getContact(user)
	if err != nil {
		return false, "", err
	}
	if cached.BusinessName != businessName {
		_, err = s.db.Exec(putBusinessNameQuery, s.JID, user, businessName)
		if err != nil {
			return false, "", err
		}
		previousName := cached.BusinessName
		cached.BusinessName = businessName
		cached.Found = true
		return true, previousName, nil
	}
	return false, "", nil
}

func (s *SQLStore) PutContactName(user types.JID, firstName, fullName string) error {
	s.contactCacheLock.Lock()
	defer s.contactCacheLock.Unlock()

	cached, err := s.getContact(user)
	if err != nil {
		return err
	}
	if cached.FirstName != firstName || cached.FullName != fullName {
		_, err = s.db.Exec(putContactNameQuery, s.JID, user, firstName, fullName)
		if err != nil {
			return err
		}
		cached.FirstName = firstName
		cached.FullName = fullName
		cached.Found = true
	}
	return nil
}

func (s *SQLStore) DeleteContactName(user types.JID) error {
	s.contactCacheLock.Lock()
	defer s.contactCacheLock.Unlock()

	_, ok := s.contactCache[user]
	if ok {
		delete(s.contactCache, user)
	}

	_, err := s.db.Exec(
		deleteContactNameQuery, s.JID, user)
	return err
}

const (
	defaultBatchSize = 300
)

func (s *SQLStore) deleteContactNamesBatch(tx execable, contacts []store.ContactEntry) error {
	values := make([]interface{}, 1, 1+len(contacts))
	queryParts := make([]string, 0, len(contacts))
	values[0] = s.JID
	placeholderSyntax := "$%d"
	if s.dialect == "sqlite3" {
		placeholderSyntax = "?%d"
	}
	i := 0
	for _, contact := range contacts {
		if contact.JID.IsEmpty() {
			s.log.Warnf("Empty contact info in mass insert: %+v", contact)
			continue
		}
		values = append(values, contact.JID.String())
		queryParts = append(queryParts, fmt.Sprintf(placeholderSyntax, i+2))
		i++
	}
	queryPartsStr := strings.Join(queryParts, ",")
	ss := fmt.Sprintf(deleteManyContactNamesQuery, queryPartsStr)
	_, err := tx.Exec(ss, values...)
	return err
}

func (s *SQLStore) putContactNamesBatch(tx execable, contacts []store.ContactEntry) error {
	values := make([]interface{}, 2, 2+len(contacts)*3)
	queryParts := make([]string, 0, len(contacts))
	values[0] = s.JID
	values[1] = true
	placeholderSyntax := "($1, $%d, $%d, $%d, $2)"
	if s.dialect == "sqlite3" {
		placeholderSyntax = "(?1, ?%d, ?%d, ?%d, ?2)"
	}
	i := 0
	for _, contact := range contacts {
		if contact.JID.IsEmpty() {
			s.log.Warnf("Empty contact info in mass insert: %+v", contact)
			continue
		}
		baseIndex := i*3 + 2
		values = append(values, contact.JID.String(), contact.FirstName, contact.FullName)
		queryParts = append(queryParts, fmt.Sprintf(placeholderSyntax, baseIndex+1, baseIndex+2, baseIndex+3))
		i++
	}
	_, err := tx.Exec(fmt.Sprintf(putManyContactNamesQuery, strings.Join(queryParts, ",")), values...)
	return err
}

func (s *SQLStore) deleteContacts(contacts []store.ContactEntry) error {
	s.log.Infof("deleting %v contacts to the db", len(contacts))
	if len(contacts) > defaultBatchSize {
		tx, err := s.db.Begin()
		if err != nil {
			return fmt.Errorf("failed to start transaction: %w", err)
		}
		for i := 0; i < len(contacts); i += defaultBatchSize {
			var contactSlice []store.ContactEntry
			if len(contacts) > i+defaultBatchSize {
				contactSlice = contacts[i : i+defaultBatchSize]
			} else {
				contactSlice = contacts[i:]
			}
			err = s.deleteContactNamesBatch(tx, contactSlice)
			if err != nil {
				_ = tx.Rollback()
				return err
			}
		}
		err = tx.Commit()
		if err != nil {
			return fmt.Errorf("failed to commit transaction: %w", err)
		}
	} else if len(contacts) > 0 {
		err := s.deleteContactNamesBatch(s.db, contacts)
		if err != nil {
			return err
		}
	} else {
		return nil
	}
	return nil
}

func (s *SQLStore) updateContacts(contacts []store.ContactEntry) error {
	s.log.Infof("inserting %v contacts to the db", len(contacts))
	if len(contacts) > defaultBatchSize {
		tx, err := s.db.Begin()
		if err != nil {
			return fmt.Errorf("failed to start transaction: %w", err)
		}
		for i := 0; i < len(contacts); i += defaultBatchSize {
			var contactSlice []store.ContactEntry
			if len(contacts) > i+defaultBatchSize {
				contactSlice = contacts[i : i+defaultBatchSize]
			} else {
				contactSlice = contacts[i:]
			}
			err = s.putContactNamesBatch(tx, contactSlice)
			if err != nil {
				_ = tx.Rollback()
				return err
			}
		}
		err = tx.Commit()
		if err != nil {
			return fmt.Errorf("failed to commit transaction: %w", err)
		}
	} else if len(contacts) > 0 {
		err := s.putContactNamesBatch(s.db, contacts)
		if err != nil {
			return err
		}
	} else {
		return nil
	}
	return nil
}

func (s *SQLStore) PutAllContactNames(contacts []store.ContactEntry) error {
	upsertContacts, deleteContacts := make([]store.ContactEntry, 0), make([]store.ContactEntry, 0)

	for _, contact := range contacts {
		if contact.Exists {
			upsertContacts = append(upsertContacts, contact)
		} else {
			deleteContacts = append(deleteContacts, contact)
		}
	}

	err := s.updateContacts(upsertContacts)
	if err != nil {
		return err
	}

	return s.deleteContacts(deleteContacts)
}

const (
	deleteManyLabelsQuery = `DELETE FROM whatsmeow_labels WHERE our_jid=$1 AND label_id IN (%s)`
	putManyLabelsQuery    = `
		INSERT INTO whatsmeow_labels (our_jid, label_id, label_name, label_color)
		VALUES %s
		ON CONFLICT (our_jid, label_id) DO UPDATE SET label_name=excluded.label_name, label_color=excluded.label_color
	`
	getLabelQuery = `
		SELECT label_name, label_color FROM whatsmeow_labels WHERE our_jid=$1 AND label_id=$2
	`
	getAllLabelsQuery = `
		SELECT label_id, label_name, label_color FROM whatsmeow_labels WHERE our_jid=$1
	`

	deleteManyLabelContactsQuery = `DELETE FROM whatsmeow_label_contacts WHERE our_jid=$1 AND (label_id, their_jid) IN (%s)`
	putManyLabelContactsQuery    = `
		INSERT INTO whatsmeow_label_contacts (our_jid, label_id, their_jid)
		VALUES %s
		ON CONFLICT (our_jid, label_id, their_jid) DO NOTHING
	`
	getAllLabelContactsQuery = `
		SELECT label_id, their_jid FROM whatsmeow_label_contacts WHERE our_jid=$1 AND label_id IN (%s)
	`
	getLabelsContactsCounts = `
		SELECT label_id, COUNT(*) as contact_count FROM whatsmeow_label_contacts WHERE our_jid=$1 GROUP BY label_id;
	`
)

func (s *SQLStore) deleteLabelsBatch(tx execable, labels []store.LabelEditEntry) error {
	values := make([]interface{}, 1, 1+len(labels))
	queryParts := make([]string, 0, len(labels))
	values[0] = s.JID
	placeholderSyntax := "$%d"
	if s.dialect == "sqlite3" {
		placeholderSyntax = "?%d"
	}
	i := 0
	for _, label := range labels {
		values = append(values, label.ID)
		queryParts = append(queryParts, fmt.Sprintf(placeholderSyntax, i+2))
		i++
	}
	queryPartsStr := strings.Join(queryParts, ",")
	ss := fmt.Sprintf(deleteManyLabelsQuery, queryPartsStr)
	_, err := tx.Exec(ss, values...)
	return err
}

func (s *SQLStore) deleteLabels(labels []store.LabelEditEntry) error {
	s.log.Infof("deleting %v labels from the db", len(labels))
	if len(labels) > defaultBatchSize {
		tx, err := s.db.Begin()
		if err != nil {
			return fmt.Errorf("failed to start transaction: %w", err)
		}
		for i := 0; i < len(labels); i += defaultBatchSize {
			var labelsSlice []store.LabelEditEntry
			if len(labels) > i+defaultBatchSize {
				labelsSlice = labels[i : i+defaultBatchSize]
			} else {
				labelsSlice = labels[i:]
			}
			err = s.deleteLabelsBatch(tx, labelsSlice)
			if err != nil {
				_ = tx.Rollback()
				return err
			}
		}
		err = tx.Commit()
		if err != nil {
			return fmt.Errorf("failed to commit transaction: %w", err)
		}
	} else if len(labels) > 0 {
		err := s.deleteLabelsBatch(s.db, labels)
		if err != nil {
			return err
		}
	} else {
		return nil
	}
	return nil
}

func (s *SQLStore) putLabelsBatch(tx execable, labels []store.LabelEditEntry) error {
	values := make([]interface{}, 1, 1+len(labels)*3)
	queryParts := make([]string, 0, len(labels))
	values[0] = s.JID
	placeholderSyntax := "($1, $%d, $%d, $%d)"
	if s.dialect == "sqlite3" {
		placeholderSyntax = "(?1, ?%d, ?%d, ?%d)"
	}
	i := 0
	for _, label := range labels {
		baseIndex := i*3 + 1
		values = append(values, label.ID, label.Name, label.Color)
		queryParts = append(queryParts, fmt.Sprintf(placeholderSyntax, baseIndex+1, baseIndex+2, baseIndex+3))
		i++
	}
	_, err := tx.Exec(fmt.Sprintf(putManyLabelsQuery, strings.Join(queryParts, ",")), values...)
	return err
}

func (s *SQLStore) updateLabels(upsertLabels []store.LabelEditEntry) error {
	s.log.Infof("inserting %v labels to the db", len(upsertLabels))
	if len(upsertLabels) > defaultBatchSize {
		tx, err := s.db.Begin()
		if err != nil {
			return fmt.Errorf("failed to start transaction: %w", err)
		}
		for i := 0; i < len(upsertLabels); i += defaultBatchSize {
			var labelsSlice []store.LabelEditEntry
			if len(upsertLabels) > i+defaultBatchSize {
				labelsSlice = upsertLabels[i : i+defaultBatchSize]
			} else {
				labelsSlice = upsertLabels[i:]
			}
			err = s.putLabelsBatch(tx, labelsSlice)
			if err != nil {
				_ = tx.Rollback()
				return err
			}
		}
		err = tx.Commit()
		if err != nil {
			return fmt.Errorf("failed to commit transaction: %w", err)
		}
	} else if len(upsertLabels) > 0 {
		err := s.putLabelsBatch(s.db, upsertLabels)
		if err != nil {
			return err
		}
	} else {
		return nil
	}
	return nil
}

func (s *SQLStore) PutAllLabels(labelEdits []store.LabelEditEntry) error {
	upsertLabels, deleteLabels := make([]store.LabelEditEntry, 0), make([]store.LabelEditEntry, 0)

	for _, labelEdit := range labelEdits {
		if labelEdit.IsDeleted {
			deleteLabels = append(deleteLabels, labelEdit)
		} else {
			upsertLabels = append(upsertLabels, labelEdit)
		}
	}

	err := s.updateLabels(upsertLabels)
	if err != nil {
		return err
	}

	return s.deleteLabels(deleteLabels)
}

func (s *SQLStore) getLabel(id int) (*types.LabelInfo, error) {
	cached, ok := s.labelsCache[id]
	if ok {
		return &cached, nil
	}

	var color int
	var name string
	err := s.db.QueryRow(getLabelQuery, s.JID, id).Scan(&name, &color)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	info := types.LabelInfo{
		Found: err == nil,
		ID:    id,
		Name:  name,
		Color: color,
	}
	s.labelsCache[id] = info
	return &info, nil
}

func (s *SQLStore) GetLabel(id int) (*types.LabelInfo, error) {
	s.labelsCacheLock.Lock()
	info, err := s.getLabel(id)
	s.labelsCacheLock.Unlock()
	if err != nil {
		return &types.LabelInfo{}, err
	}
	return info, nil
}

func (s *SQLStore) GetAllLabels() (map[int]types.LabelInfo, error) {
	s.labelsCacheLock.Lock()
	defer s.labelsCacheLock.Unlock()

	rows, err := s.db.Query(getAllLabelsQuery, s.JID)
	if err != nil {
		return nil, err
	}
	output := make(map[int]types.LabelInfo)
	for rows.Next() {
		var id, color int
		var name string
		err = rows.Scan(&id, &name, &color)
		if err != nil {
			return nil, fmt.Errorf("error scanning row: %w", err)
		}
		info := types.LabelInfo{
			Found: true,
			ID:    id,
			Name:  name,
			Color: color,
		}
		output[id] = info
		s.labelsCache[id] = info
	}
	return output, nil
}

func (s *SQLStore) GetLabelsContactsCounts() (map[int]int, error) {
	rows, err := s.db.Query(getLabelsContactsCounts, s.JID)
	if err != nil {
		return nil, err
	}
	output := make(map[int]int)
	for rows.Next() {
		var id, count int
		err = rows.Scan(&id, &count)
		if err != nil {
			return nil, fmt.Errorf("error scanning row: %w", err)
		}
		output[id] = count
	}
	return output, nil
}

func (s *SQLStore) deleteLabelContactsBatch(tx execable, labelContacts []store.LabelContactEntry) error {
	values := make([]interface{}, 1, 1+len(labelContacts))
	queryParts := make([]string, 0, len(labelContacts))
	values[0] = s.JID
	placeholderSyntax := "($%d, $%d)"
	if s.dialect == "sqlite3" {
		placeholderSyntax = "(?%d, ?%d)"
	}
	i := 0
	for _, labelContact := range labelContacts {
		values = append(values, labelContact.LabelID, labelContact.JID.String())
		queryParts = append(queryParts, fmt.Sprintf(placeholderSyntax, i+2, i+3))
		i += 2
	}
	queryPartsStr := strings.Join(queryParts, ",")
	ss := fmt.Sprintf(deleteManyLabelContactsQuery, queryPartsStr)
	_, err := tx.Exec(ss, values...)
	return err
}

func (s *SQLStore) deleteLabelContacts(labelContacts []store.LabelContactEntry) error {
	s.log.Infof("deleting %v label contacts from the db", len(labelContacts))
	if len(labelContacts) > defaultBatchSize {
		tx, err := s.db.Begin()
		if err != nil {
			return fmt.Errorf("failed to start transaction: %w", err)
		}
		for i := 0; i < len(labelContacts); i += defaultBatchSize {
			var labelContactsSlice []store.LabelContactEntry
			if len(labelContacts) > i+defaultBatchSize {
				labelContactsSlice = labelContacts[i : i+defaultBatchSize]
			} else {
				labelContactsSlice = labelContacts[i:]
			}
			err = s.deleteLabelContactsBatch(tx, labelContactsSlice)
			if err != nil {
				_ = tx.Rollback()
				return err
			}
		}
		err = tx.Commit()
		if err != nil {
			return fmt.Errorf("failed to commit transaction: %w", err)
		}
	} else if len(labelContacts) > 0 {
		err := s.deleteLabelContactsBatch(s.db, labelContacts)
		if err != nil {
			return err
		}
	} else {
		return nil
	}
	return nil
}

func (s *SQLStore) updateLabelContactsBatch(tx execable, labelContacts []store.LabelContactEntry) error {
	values := make([]interface{}, 1, 1+len(labelContacts)*3)
	queryParts := make([]string, 0, len(labelContacts))
	values[0] = s.JID
	placeholderSyntax := "($1, $%d, $%d)"
	if s.dialect == "sqlite3" {
		placeholderSyntax = "(?1, ?%d, ?%d)"
	}
	i := 0
	for _, labelContact := range labelContacts {
		baseIndex := i*2 + 1
		values = append(values, labelContact.LabelID, labelContact.JID)
		queryParts = append(queryParts, fmt.Sprintf(placeholderSyntax, baseIndex+1, baseIndex+2))
		i++
	}
	_, err := tx.Exec(fmt.Sprintf(putManyLabelContactsQuery, strings.Join(queryParts, ",")), values...)
	return err
}

func (s *SQLStore) updateLabelContacts(labelContacts []store.LabelContactEntry) error {
	s.log.Infof("inserting %v label contacts to the db", len(labelContacts))
	if len(labelContacts) > defaultBatchSize {
		tx, err := s.db.Begin()
		if err != nil {
			return fmt.Errorf("failed to start transaction: %w", err)
		}
		for i := 0; i < len(labelContacts); i += defaultBatchSize {
			var labelContactsSlice []store.LabelContactEntry
			if len(labelContacts) > i+defaultBatchSize {
				labelContactsSlice = labelContacts[i : i+defaultBatchSize]
			} else {
				labelContactsSlice = labelContacts[i:]
			}
			err = s.updateLabelContactsBatch(tx, labelContactsSlice)
			if err != nil {
				_ = tx.Rollback()
				return err
			}
		}
		err = tx.Commit()
		if err != nil {
			return fmt.Errorf("failed to commit transaction: %w", err)
		}
	} else if len(labelContacts) > 0 {
		err := s.updateLabelContactsBatch(s.db, labelContacts)
		if err != nil {
			return err
		}
	} else {
		return nil
	}
	return nil
}

func (s *SQLStore) PutAllLabelContacts(labelContacts []store.LabelContactEntry) error {
	upsertLabelContacts, deleteLabelContactss := make([]store.LabelContactEntry, 0), make([]store.LabelContactEntry, 0)

	for _, labelContact := range labelContacts {
		if labelContact.IsLabeled {
			upsertLabelContacts = append(upsertLabelContacts, labelContact)
		} else {
			deleteLabelContactss = append(deleteLabelContactss, labelContact)
		}
	}

	err := s.updateLabelContacts(upsertLabelContacts)
	if err != nil {
		return err
	}

	return s.deleteLabelContacts(deleteLabelContactss)
}

func (s *SQLStore) GetAllLabelContacts(id int, ids ...int) (map[int][]types.JID, error) {
	ids = append(ids, id)

	values := make([]interface{}, 1, 1+len(ids))
	queryParts := make([]string, 0, len(ids))
	values[0] = s.JID
	placeholderSyntax := "$%d"
	if s.dialect == "sqlite3" {
		placeholderSyntax = "?%d"
	}
	i := 0
	for _, labelID := range ids {
		values = append(values, labelID)
		queryParts = append(queryParts, fmt.Sprintf(placeholderSyntax, i+2))
		i++
	}
	queryPartsStr := strings.Join(queryParts, ",")
	queryStr := fmt.Sprintf(getAllLabelContactsQuery, queryPartsStr)

	rows, err := s.db.Query(queryStr, values...)
	if err != nil {
		return nil, err
	}
	output := make(map[int][]types.JID)
	for rows.Next() {
		var labelID int
		var jid types.JID
		err = rows.Scan(&labelID, &jid)
		if err != nil {
			return nil, fmt.Errorf("error scanning row: %w", err)
		}

		if _, ok := output[labelID]; !ok {
			output[labelID] = make([]types.JID, 0)
		}

		output[labelID] = append(output[labelID], jid)
	}
	return output, nil
}

func (s *SQLStore) getContact(user types.JID) (*types.ContactInfo, error) {
	cached, ok := s.contactCache[user]
	if ok {
		return cached, nil
	}

	var first, full, push, business sql.NullString
	var isPhoneContact bool
	err := s.db.QueryRow(getContactQuery, s.JID, user).Scan(&first, &full, &push, &business, &isPhoneContact)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	info := &types.ContactInfo{
		Found:          err == nil,
		IsPhoneContact: isPhoneContact,
		FirstName:      first.String,
		FullName:       full.String,
		PushName:       push.String,
		BusinessName:   business.String,
	}
	s.contactCache[user] = info
	return info, nil
}

func (s *SQLStore) GetContact(user types.JID) (types.ContactInfo, error) {
	s.contactCacheLock.Lock()
	info, err := s.getContact(user)
	s.contactCacheLock.Unlock()
	if err != nil {
		return types.ContactInfo{}, err
	}
	return *info, nil
}

func (s *SQLStore) GetAllContacts() (map[types.JID]types.ContactInfo, error) {
	s.contactCacheLock.Lock()
	defer s.contactCacheLock.Unlock()
	rows, err := s.db.Query(getAllContactsQuery, s.JID)
	if err != nil {
		return nil, err
	}
	output := make(map[types.JID]types.ContactInfo, len(s.contactCache))
	for rows.Next() {
		var jid types.JID
		var first, full, push, business sql.NullString
		var isPhoneContact bool
		err = rows.Scan(&jid, &first, &full, &push, &business, &isPhoneContact)
		if err != nil {
			return nil, fmt.Errorf("error scanning row: %w", err)
		}
		info := types.ContactInfo{
			Found:          true,
			IsPhoneContact: isPhoneContact,
			FirstName:      first.String,
			FullName:       full.String,
			PushName:       push.String,
			BusinessName:   business.String,
		}
		output[jid] = info
		s.contactCache[jid] = &info
	}
	return output, nil
}

const (
	putManySettingArchivedQuery = `
		INSERT INTO whatsmeow_chat_settings (our_jid, chat_jid, archived, archived_timestamp)
		VALUES %s
		ON CONFLICT (our_jid, chat_jid) DO UPDATE SET archived=excluded.archived, archived_timestamp=excluded.archived_timestamp
	`

	putChatSettingArchivedQuery = `
		INSERT INTO whatsmeow_chat_settings (our_jid, chat_jid, archived, archived_timestamp) VALUES ($1, $2, $3, $4)
		ON CONFLICT (our_jid, chat_jid) DO UPDATE SET archived=excluded.archived, archived_timestamp=excluded.archived_timestamp
	`

	getChatSettingsArchivedQuery = `
		SELECT archived, archived_timestamp FROM whatsmeow_chat_settings WHERE our_jid=$1 AND chat_jid=$2
	`

	putChatSettingQuery = `
		INSERT INTO whatsmeow_chat_settings (our_jid, chat_jid, %[1]s) VALUES ($1, $2, $3)
		ON CONFLICT (our_jid, chat_jid) DO UPDATE SET %[1]s=excluded.%[1]s
	`
	getChatSettingsQuery = `
		SELECT muted_until, pinned, archived FROM whatsmeow_chat_settings WHERE our_jid=$1 AND chat_jid=$2
	`
)

func (s *SQLStore) PutMutedUntil(chat types.JID, mutedUntil time.Time) error {
	var val int64
	if !mutedUntil.IsZero() {
		val = mutedUntil.Unix()
	}
	_, err := s.db.Exec(fmt.Sprintf(putChatSettingQuery, "muted_until"), s.JID, chat, val)
	return err
}

func (s *SQLStore) PutPinned(chat types.JID, pinned bool) error {
	_, err := s.db.Exec(fmt.Sprintf(putChatSettingQuery, "pinned"), s.JID, chat, pinned)
	return err
}

func (s *SQLStore) PutArchived(chat types.JID, archiveEntry store.ArchivedEntry) error {
	s.archivedCacheLock.Lock()
	defer s.archivedCacheLock.Unlock()
	s.archivedCache[archiveEntry.JID] = &archiveEntry

	_, err := s.db.Exec(putChatSettingArchivedQuery, s.JID, chat, archiveEntry.Archived, archiveEntry.ArchivedTime)
	return err
}

func (s *SQLStore) putArchivesBatch(tx execable, archives []store.ArchivedEntry) error {
	values := make([]interface{}, 1, 1+len(archives)*3)
	queryParts := make([]string, 0, len(archives))
	values[0] = s.JID
	placeholderSyntax := "($1, $%d, $%d, $%d)"
	if s.dialect == "sqlite3" {
		placeholderSyntax = "(?1, ?%d, ?%d, ?%d)"
	}
	i := 0
	handledArchives := make(map[types.JID]struct{}, len(archives))
	for _, archive := range archives {
		if archive.JID.IsEmpty() {
			s.log.Warnf("Empty archive JID in mass insert: %+v", archive)
			continue
		}
		// The whole query will break if there are duplicates, so make sure there aren't any duplicates
		_, alreadyHandled := handledArchives[archive.JID]
		if alreadyHandled {
			s.log.Warnf("Duplicate archive info for %s in mass insert", archive.JID)
			continue
		}
		handledArchives[archive.JID] = struct{}{}
		baseIndex := i*3 + 1
		values = append(values, archive.JID.String(), archive.Archived, archive.ArchivedTime.UnixMilli())
		queryParts = append(queryParts, fmt.Sprintf(placeholderSyntax, baseIndex+1, baseIndex+2, baseIndex+3))
		i++
	}

	_, err := tx.Exec(fmt.Sprintf(putManySettingArchivedQuery, strings.Join(queryParts, ",")), values...)
	return err
}

func (s *SQLStore) PutAllArchives(archives []store.ArchivedEntry) error {
	s.archivedCacheLock.Lock()
	defer s.archivedCacheLock.Unlock()

	for _, archiveEntry := range archives {
		cur := archiveEntry
		s.archivedCache[archiveEntry.JID] = &cur
	}

	s.log.Infof("inserting %v archives to the db", len(archives))
	if len(archives) > defaultBatchSize {
		tx, err := s.db.Begin()
		if err != nil {
			return fmt.Errorf("failed to start transaction: %w", err)
		}
		for i := 0; i < len(archives); i += defaultBatchSize {
			var archivesSlice []store.ArchivedEntry
			if len(archives) > i+defaultBatchSize {
				archivesSlice = archives[i : i+defaultBatchSize]
			} else {
				archivesSlice = archives[i:]
			}
			err = s.putArchivesBatch(tx, archivesSlice)
			if err != nil {
				_ = tx.Rollback()
				return err
			}
		}
		err = tx.Commit()
		if err != nil {
			return fmt.Errorf("failed to commit transaction: %w", err)
		}
	} else if len(archives) > 0 {
		err := s.putArchivesBatch(s.db, archives)
		if err != nil {
			return err
		}
	} else {
		return nil
	}
	return nil
}

func (s *SQLStore) GetChatSettingsArchived(chat types.JID) (archiveEntry store.ArchivedEntry, ok bool, err error) {
	s.archivedCacheLock.RLock()
	cached, ok := s.archivedCache[chat]
	if ok {
		defer s.archivedCacheLock.RUnlock()
		return *cached, true, nil
	}
	s.archivedCacheLock.RUnlock()

	s.archivedCacheLock.Lock()
	s.archivedCacheLock.Unlock()

	var archiveTimestamp int64
	err = s.db.QueryRow(getChatSettingsArchivedQuery, s.JID, chat).Scan(&archiveEntry.Archived, &archiveTimestamp)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return store.ArchivedEntry{}, false, nil
		}

		return store.ArchivedEntry{}, false, err
	}

	archiveEntry.JID = chat
	archiveEntry.ArchivedTime = time.UnixMilli(archiveTimestamp)
	s.archivedCache[chat] = &archiveEntry

	return archiveEntry, true, nil
}

func (s *SQLStore) GetChatSettings(chat types.JID) (settings types.LocalChatSettings, err error) {
	var mutedUntil int64
	err = s.db.QueryRow(getChatSettingsQuery, s.JID, chat).Scan(&mutedUntil, &settings.Pinned, &settings.Archived)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	} else if err != nil {
		return
	} else {
		settings.Found = true
	}
	if mutedUntil != 0 {
		settings.MutedUntil = time.Unix(mutedUntil, 0)
	}
	return
}

const (
	putMsgSecret = `
		INSERT INTO whatsmeow_message_secrets (our_jid, chat_jid, sender_jid, message_id, key)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (our_jid, chat_jid, sender_jid, message_id) DO NOTHING
	`
	getMsgSecret = `
		SELECT key FROM whatsmeow_message_secrets WHERE our_jid=$1 AND chat_jid=$2 AND sender_jid=$3 AND message_id=$4
	`
)

func (s *SQLStore) PutMessageSecrets(inserts []store.MessageSecretInsert) (err error) {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	for _, insert := range inserts {
		_, err = tx.Exec(putMsgSecret, s.JID, insert.Chat.ToNonAD(), insert.Sender.ToNonAD(), insert.ID, insert.Secret)
	}
	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return
}

func (s *SQLStore) PutMessageSecret(chat, sender types.JID, id types.MessageID, secret []byte) (err error) {
	_, err = s.db.Exec(putMsgSecret, s.JID, chat.ToNonAD(), sender.ToNonAD(), id, secret)
	return
}

func (s *SQLStore) GetMessageSecret(chat, sender types.JID, id types.MessageID) (secret []byte, err error) {
	err = s.db.QueryRow(getMsgSecret, s.JID, chat.ToNonAD(), sender.ToNonAD(), id).Scan(&secret)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	return
}

const (
	putPrivacyTokens = `
		INSERT INTO whatsmeow_privacy_tokens (our_jid, their_jid, token, timestamp)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (our_jid, their_jid) DO UPDATE SET token=EXCLUDED.token, timestamp=EXCLUDED.timestamp
	`
	getPrivacyToken = `SELECT token, timestamp FROM whatsmeow_privacy_tokens WHERE our_jid=$1 AND their_jid=$2`
)

func (s *SQLStore) PutPrivacyTokens(tokens ...store.PrivacyToken) error {
	args := make([]any, 1+len(tokens)*3)
	placeholders := make([]string, len(tokens))
	args[0] = s.JID
	for i, token := range tokens {
		args[i*3+1] = token.User.ToNonAD().String()
		args[i*3+2] = token.Token
		args[i*3+3] = token.Timestamp.Unix()
		placeholders[i] = fmt.Sprintf("($1, $%d, $%d, $%d)", i*3+2, i*3+3, i*3+4)
	}
	query := strings.ReplaceAll(putPrivacyTokens, "($1, $2, $3, $4)", strings.Join(placeholders, ","))
	_, err := s.db.Exec(query, args...)
	return err
}

func (s *SQLStore) GetPrivacyToken(user types.JID) (*store.PrivacyToken, error) {
	var token store.PrivacyToken
	token.User = user.ToNonAD()
	var ts int64
	err := s.db.QueryRow(getPrivacyToken, s.JID, token.User).Scan(&token.Token, &ts)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, err
	} else {
		token.Timestamp = time.Unix(ts, 0)
		return &token, nil
	}
}
