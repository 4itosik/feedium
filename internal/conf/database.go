package conf

import "errors"

func ValidateAndNormalizeDatabase(db *Database) error {
	if db.GetHost() == "" {
		db.Host = "127.0.0.1"
	}
	if db.GetPort() == 0 {
		db.Port = 5432
	}
	if db.GetDatabase() == "" {
		return errors.New("database is required")
	}
	if db.GetUser() == "" {
		return errors.New("user is required")
	}
	if db.GetPassword() == "" {
		return errors.New("password is required")
	}
	if db.GetSslmode() == "" {
		return errors.New("sslmode is required")
	}
	return nil
}
