package pattern

import (
	"database/sql"
	"log"

	"github.com/hidori/go-test-pgssi/txm"
)

type PatternBase struct {
	connectionString string
	patternName      string
	looserReady      chan struct{}
	winnerReady      chan struct{}
	proceedSignal    chan struct{}
	looserWriteDone  chan struct{}
	winnerWriteDone  chan struct{}
	commitSignal     chan struct{}
}

func NewPatternBase(connectionString, patternName string) PatternBase {
	return PatternBase{
		connectionString: connectionString,
		patternName:      patternName,
		looserReady:      make(chan struct{}),
		winnerReady:      make(chan struct{}),
		proceedSignal:    make(chan struct{}),
		looserWriteDone:  make(chan struct{}),
		winnerWriteDone:  make(chan struct{}),
		commitSignal:     make(chan struct{}),
	}
}

func (b *PatternBase) Do(db *sql.DB, done chan struct{},
	looserFn func(tx *sql.Tx, done chan struct{}) error,
	winnerFn func(tx *sql.Tx, done chan struct{}) error) {
	log.Println("Orchestrator: Starting looser & winner...")

	db1, err := sql.Open("postgres", b.connectionString)
	if err != nil {
		log.Printf("**** Failed to create db1 connection: %v", err)
		done <- struct{}{}
		done <- struct{}{}
		return
	}
	defer db1.Close()

	db2, err := sql.Open("postgres", b.connectionString)
	if err != nil {
		log.Printf("**** Failed to create db2 connection: %v", err)
		done <- struct{}{}
		done <- struct{}{}
		return
	}
	defer db2.Close()

	go func() {
		log.Println("Looser: Starting...")
		err := txm.TransactionWithSerializable(db1, b.patternName, "Looser", func(tx *sql.Tx) error {
			return looserFn(tx, done)
		})
		if err != nil {
			log.Printf("**** Looser %s failed: %v", b.patternName, err)
		}
	}()

	go func() {
		log.Println("Winner: Starting...")
		err := txm.TransactionWithSerializable(db2, b.patternName, "Winner", func(tx *sql.Tx) error {
			return winnerFn(tx, done)
		})
		if err != nil {
			log.Printf("**** Winner %s failed: %v", b.patternName, err)
		}
	}()

	log.Println("Orchestrator: Waiting for looser ready signal...")
	<-b.looserReady
	log.Println("Orchestrator: Waiting for winner ready signal...")
	<-b.winnerReady
	log.Println("Orchestrator: Both ready, closing proceed signal...")
	close(b.proceedSignal)
	log.Println("Orchestrator: Proceed signal closed")

	log.Println("Orchestrator: Waiting for looser write done signal...")
	<-b.looserWriteDone
	log.Println("Orchestrator: Waiting for winner write done signal...")
	<-b.winnerWriteDone
	log.Println("Orchestrator: Both writes done, closing commit signal...")
	close(b.commitSignal)
	log.Println("Orchestrator: Commit signal closed")
}
