package fsm

import (
	"encoding/json"
	"errors"
	"github.com/dgraph-io/badger/v3"
	"github.com/mitchellh/mapstructure"
	"github.com/narvikd/errorskit"
	"nubewatch/register/registermodel"
	"time"
)

func (dbFSM DatabaseFSM) addService(serviceName string, v any) (string, error) {
	var servers []registermodel.Server
	var inputServer registermodel.Server
	dbResultValue := make([]byte, 0)
	isPut := false
	foundDeadServer := false

	// It's already a pointer, error can be ignored
	decoder, _ := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		DecodeHook: mapstructure.StringToTimeHookFunc(time.RFC3339),
		Result:     &inputServer,
		TagName:    "json",
	})
	errDecodeInputServer := decoder.Decode(v)
	if errDecodeInputServer != nil {
		return "", errorskit.Wrap(errDecodeInputServer, "couldn't decode input server")
	}

	txn := dbFSM.db.NewTransaction(true)
	defer txn.Discard()

	dbResult, errGet := txn.Get([]byte(serviceName))
	if errGet != nil && !errors.Is(errGet, badger.ErrKeyNotFound) {
		return "", errGet
	}

	if dbResult != nil {
		errDBResultValue := dbResult.Value(func(val []byte) error {
			dbResultValue = append(dbResultValue, val...)
			return nil
		})
		if errDBResultValue != nil {
			return "", errDBResultValue
		}

		if dbResultValue == nil || len(dbResultValue) <= 0 {
			return "", errors.New("no result for key")
		}

		errUnmarshalDBResult := json.Unmarshal(dbResultValue, &servers)
		if errUnmarshalDBResult != nil {
			return "", errorskit.Wrap(errUnmarshalDBResult, "couldn't unmarshal get results from DB")
		}
	}

	for i := 0; i < len(servers); i++ {
		// If same ID: Replace
		if servers[i].Host == inputServer.Host {
			isPut = true
			if servers[i].Alive {
				return "", errors.New("couldn't register server because an already alive server existed with that host")
			}
			inputServer.ID = servers[i].ID
			servers[i] = inputServer
			break
		}
	}

	if !isPut {
		for i := 0; i < len(servers); i++ {
			if !servers[i].Alive {
				foundDeadServer = true
				inputServer.ID = servers[i].ID
				servers[i] = inputServer
				break
			}
		}
		if !foundDeadServer {
			servers = append(servers, inputServer)
		}
	}

	dbSetValue, errMarshal := json.Marshal(servers)
	if errMarshal != nil {
		return "", errorskit.Wrap(errMarshal, "couldn't marshal value on set addService")
	}

	if len(dbSetValue) <= 0 {
		return "", errors.New("value was empty")
	}

	errSet := txn.Set([]byte(serviceName), dbSetValue)
	if errSet != nil {
		return "", errSet
	}

	errCommit := txn.Commit()
	if errCommit != nil {
		return "", errorskit.Wrap(errCommit, "couldn't commit transaction")
	}

	return inputServer.ID, nil
}

func (dbFSM DatabaseFSM) GetServers(ServiceName string) ([]registermodel.Server, error) {
	var servers []registermodel.Server

	get, errGet := dbFSM.Get(ServiceName)
	if errGet != nil && !errors.Is(errGet, badger.ErrKeyNotFound) {
		return nil, errorskit.Wrap(errGet, "couldn't get servers")
	}

	// It's already a pointer, error can be ignored
	decoder, _ := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		DecodeHook: mapstructure.StringToTimeHookFunc(time.RFC3339),
		Result:     &servers,
		TagName:    "json",
	})
	errDecodeInputServer := decoder.Decode(get)
	if errDecodeInputServer != nil {
		return nil, errorskit.Wrap(errDecodeInputServer, "couldn't decode servers from db")
	}

	return servers, nil
}

func (dbFSM DatabaseFSM) GetAllServers() ([]registermodel.Server, error) {
	var keys []string
	var servers []registermodel.Server

	txn := dbFSM.db.NewTransaction(false)
	defer txn.Discard()

	it := txn.NewIterator(badger.DefaultIteratorOptions)
	defer it.Close()

	for it.Rewind(); it.Valid(); it.Next() {
		key := it.Item().KeyCopy(nil)
		keys = append(keys, string(key))
	}

	for _, key := range keys {
		var keyServers []registermodel.Server

		get, errGet := dbFSM.Get(key)
		if errGet != nil && !errors.Is(errGet, badger.ErrKeyNotFound) {
			return nil, errorskit.Wrap(errGet, "couldn't get servers")
		}

		// It's already a pointer, error can be ignored
		decoder, _ := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
			DecodeHook: mapstructure.StringToTimeHookFunc(time.RFC3339),
			Result:     &keyServers,
			TagName:    "json",
		})
		errDecodeInputServer := decoder.Decode(get)
		if errDecodeInputServer != nil {
			return nil, errorskit.Wrap(errDecodeInputServer, "couldn't decode servers from db")
		}

		for _, server := range keyServers {
			servers = append(servers, server)
		}
	}

	return servers, nil
}

func (dbFSM DatabaseFSM) setNotAliveServer(serviceName string, serverIDAny any) error {
	var servers []registermodel.Server
	dbResultValue := make([]byte, 0)

	txn := dbFSM.db.NewTransaction(true)
	defer txn.Discard()

	dbResult, errGet := txn.Get([]byte(serviceName))
	if errGet != nil {
		return errGet
	}

	errDBResultValue := dbResult.Value(func(val []byte) error {
		dbResultValue = append(dbResultValue, val...)
		return nil
	})
	if errDBResultValue != nil {
		return errDBResultValue
	}

	if dbResultValue == nil || len(dbResultValue) <= 0 {
		return errors.New("no result for key")
	}

	errUnmarshalDBResult := json.Unmarshal(dbResultValue, &servers)
	if errUnmarshalDBResult != nil {
		return errorskit.Wrap(errUnmarshalDBResult, "couldn't unmarshal get results from DB")
	}

	for i := 0; i < len(servers); i++ {
		if servers[i].ID == serverIDAny.(string) {
			servers[i].Alive = false
			break
		}
	}

	dbSetValue, errMarshal := json.Marshal(servers)
	if errMarshal != nil {
		return errorskit.Wrap(errMarshal, "couldn't marshal value on set setNotAliveServer")
	}

	if len(dbSetValue) <= 0 {
		return errors.New("value was empty")
	}

	errSet := txn.Set([]byte(serviceName), dbSetValue)
	if errSet != nil {
		return errSet
	}

	errCommit := txn.Commit()
	if errCommit != nil {
		return errorskit.Wrap(errCommit, "couldn't commit transaction")
	}

	return nil
}

func (dbFSM DatabaseFSM) updateLastContact(serviceName string, v any) error {
	var servers []registermodel.Server
	var inputServer registermodel.Server
	dbResultValue := make([]byte, 0)
	found := false

	// It's already a pointer, error can be ignored
	decoder, _ := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		DecodeHook: mapstructure.StringToTimeHookFunc(time.RFC3339),
		Result:     &inputServer,
		TagName:    "json",
	})
	errDecodeInputServer := decoder.Decode(v)
	if errDecodeInputServer != nil {
		return errorskit.Wrap(errDecodeInputServer, "couldn't decode input server")
	}

	txn := dbFSM.db.NewTransaction(true)
	defer txn.Discard()

	dbResult, errGet := txn.Get([]byte(serviceName))
	if errGet != nil {
		return errGet
	}

	errDBResultValue := dbResult.Value(func(val []byte) error {
		dbResultValue = append(dbResultValue, val...)
		return nil
	})
	if errDBResultValue != nil {
		return errDBResultValue
	}

	if dbResultValue == nil || len(dbResultValue) <= 0 {
		return errors.New("no result for key")
	}

	errUnmarshalDBResult := json.Unmarshal(dbResultValue, &servers)
	if errUnmarshalDBResult != nil {
		return errorskit.Wrap(errUnmarshalDBResult, "couldn't unmarshal get results from DB")
	}

	for i := 0; i < len(servers); i++ {
		if servers[i].ID == inputServer.ID {
			found = true
			servers[i].LastContact = time.Now()
			break
		}
	}

	if !found {
		return errors.New("server not found")
	}

	dbSetValue, errMarshal := json.Marshal(servers)
	if errMarshal != nil {
		return errorskit.Wrap(errMarshal, "couldn't marshal value on set setNotAliveServer")
	}

	if len(dbSetValue) <= 0 {
		return errors.New("value was empty")
	}

	errSet := txn.Set([]byte(serviceName), dbSetValue)
	if errSet != nil {
		return errSet
	}

	errCommit := txn.Commit()
	if errCommit != nil {
		return errorskit.Wrap(errCommit, "couldn't commit transaction")
	}

	return nil

}

// Get is a DatabaseFSM's method which gets a value from a key from the LOCAL NODE.
//
// This method isn't committed since there's no need for it.
func (dbFSM DatabaseFSM) Get(k string) (any, error) {
	var result any
	dbResultValue := make([]byte, 0)

	txn := dbFSM.db.NewTransaction(false)
	defer txn.Discard()
	dbResult, errGet := txn.Get([]byte(k))
	if errGet != nil {
		return nil, errGet
	}

	errDBResultValue := dbResult.Value(func(val []byte) error {
		dbResultValue = append(dbResultValue, val...)
		return nil
	})
	if errDBResultValue != nil {
		return nil, errDBResultValue
	}

	if dbResultValue == nil || len(dbResultValue) <= 0 {
		return nil, errors.New("no result for key")
	}

	errUnmarshal := json.Unmarshal(dbResultValue, &result)
	if errUnmarshal != nil {
		return nil, errorskit.Wrap(errUnmarshal, "couldn't unmarshal get results from DB")
	}

	errCommit := txn.Commit()
	if errCommit != nil {
		return nil, errorskit.Wrap(errCommit, "couldn't commit transaction")
	}

	return result, nil
}

// set is a DatabaseFSM's method which adds a key-value pair to the database.
func (dbFSM DatabaseFSM) set(k string, value any) error {
	dbValue, errMarshal := json.Marshal(value)
	if errMarshal != nil {
		return errorskit.Wrap(errMarshal, "couldn't marshal value on set")
	}

	if dbValue == nil || len(dbValue) <= 0 {
		return errors.New("value was empty")
	}

	txn := dbFSM.db.NewTransaction(true)
	defer txn.Discard()
	errSet := txn.Set([]byte(k), dbValue)
	if errSet != nil {
		return errSet
	}

	errCommit := txn.Commit()
	if errCommit != nil {
		return errorskit.Wrap(errCommit, "couldn't commit transaction")
	}

	return nil
}
