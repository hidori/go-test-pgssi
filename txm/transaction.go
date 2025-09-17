package txm

import (
	"database/sql"
	"log"

	"github.com/pkg/errors"
)

func Transaction(db *sql.DB, patternName, executorName string, fn func(tx *sql.Tx) error) error {
	tx, err := db.Begin()
	if err != nil {
		log.Printf("[%s:%s] Failed to begin transaction: %v", patternName, executorName, err)
		return errors.WithStack(err)
	}

	err = fn(tx)
	if err != nil {
		rollbackErr := tx.Rollback()
		if rollbackErr != nil {
			log.Printf("[%s:%s] Failed to rollback transaction: %v", patternName, executorName, rollbackErr)
		}
		log.Printf("[%s:%s] Transaction function returned error: %v", patternName, executorName, err)
		return errors.WithStack(err)
	}

	err = tx.Commit()
	if err != nil {
		log.Printf("[%s:%s] Failed to commit transaction: %v", patternName, executorName, err)
		return errors.WithStack(err)
	}

	return nil
}

func TransactionWithSerializable(db *sql.DB, patternName, executorName string, fn func(tx *sql.Tx) error) error {
	tx, err := db.Begin()
	if err != nil {
		log.Printf("[%s:%s] Failed to begin transaction: %v", patternName, executorName, err)
		return errors.WithStack(err)
	}

	_, err = tx.Exec("SET TRANSACTION ISOLATION LEVEL SERIALIZABLE;")
	if err != nil {
		rollbackErr := tx.Rollback()
		if rollbackErr != nil {
			log.Printf("[%s:%s] Failed to rollback transaction: %v", patternName, executorName, rollbackErr)
		}
		log.Panicf("[%s:%s] Failed to set transaction isolation level: %v", patternName, executorName, err)
		return errors.WithStack(err)
	}

	err = fn(tx)
	if err != nil {
		rollbackErr := tx.Rollback()
		if rollbackErr != nil {
			log.Printf("[%s:%s] Failed to rollback transaction: %v", patternName, executorName, rollbackErr)
		}
		log.Printf("[%s:%s] Transaction function returned error: %v", patternName, executorName, err)
		return errors.WithStack(err)
	}

	err = tx.Commit()
	if err != nil {
		log.Printf("[%s:%s] Failed to commit transaction: %v", patternName, executorName, err)
		return errors.WithStack(err)
	}

	return nil
}
