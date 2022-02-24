package postgres

import "github.com/ltcsuite/ltcwallet/walletdb"

type Fixture interface {
	DB() walletdb.DB
	Dump() (map[string]interface{}, error)
}
