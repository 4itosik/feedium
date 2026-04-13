package conf_test

import (
	"testing"

	"github.com/4itosik/feedium/internal/conf"
	"github.com/stretchr/testify/require"
)

func TestValidateAndNormalizeDatabase_HappyPath(t *testing.T) {
	db := &conf.Database{
		Host:     "localhost",
		Port:     5432,
		Database: "testdb",
		User:     "testuser",
		Password: "testpass",
		Sslmode:  "disable",
	}

	err := conf.ValidateAndNormalizeDatabase(db)
	require.NoError(t, err)
	require.Equal(t, "localhost", db.GetHost())
	require.Equal(t, int32(5432), db.GetPort())
	require.Equal(t, "testdb", db.GetDatabase())
	require.Equal(t, "testuser", db.GetUser())
	require.Equal(t, "testpass", db.GetPassword())
	require.Equal(t, "disable", db.GetSslmode())
}

func TestValidateAndNormalizeDatabase_Defaults(t *testing.T) {
	db := &conf.Database{
		Database: "testdb",
		User:     "testuser",
		Password: "testpass",
		Sslmode:  "disable",
	}

	err := conf.ValidateAndNormalizeDatabase(db)
	require.NoError(t, err)
	require.Equal(t, "127.0.0.1", db.GetHost())
	require.Equal(t, int32(5432), db.GetPort())
}

func TestValidateAndNormalizeDatabase_MissingDatabase(t *testing.T) {
	db := &conf.Database{
		User:     "testuser",
		Password: "testpass",
		Sslmode:  "disable",
	}

	err := conf.ValidateAndNormalizeDatabase(db)
	require.Error(t, err)
	require.Contains(t, err.Error(), "database")
}

func TestValidateAndNormalizeDatabase_MissingUser(t *testing.T) {
	db := &conf.Database{
		Database: "testdb",
		Password: "testpass",
		Sslmode:  "disable",
	}

	err := conf.ValidateAndNormalizeDatabase(db)
	require.Error(t, err)
	require.Contains(t, err.Error(), "user")
}

func TestValidateAndNormalizeDatabase_MissingPassword(t *testing.T) {
	db := &conf.Database{
		Database: "testdb",
		User:     "testuser",
		Sslmode:  "disable",
	}

	err := conf.ValidateAndNormalizeDatabase(db)
	require.Error(t, err)
	require.Contains(t, err.Error(), "password")
}

func TestValidateAndNormalizeDatabase_MissingSslmode(t *testing.T) {
	db := &conf.Database{
		Database: "testdb",
		User:     "testuser",
		Password: "testpass",
	}

	err := conf.ValidateAndNormalizeDatabase(db)
	require.Error(t, err)
	require.Contains(t, err.Error(), "sslmode")
}

func TestValidateAndNormalizeDatabase_AllDefaults(t *testing.T) {
	db := &conf.Database{
		Database: "testdb",
		User:     "testuser",
		Password: "testpass",
		Sslmode:  "disable",
	}

	err := conf.ValidateAndNormalizeDatabase(db)
	require.NoError(t, err)
	require.Equal(t, "127.0.0.1", db.GetHost())
	require.Equal(t, int32(5432), db.GetPort())
}
