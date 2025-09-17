package pattern

import (
	"database/sql"
	"log"
)

/*
	範囲クエリで新規レコードが「幻」のように出現する競合

	SSIがファントムリード現象を検出するかテスト：
	1. LooserとWinnerの両方がA-M範囲のレコード数をカウント（読み取りセット確立）
	2. Looserが'Charlie'を挿入、Winnerが'Diana'を挿入（どちらもA-M範囲内）
	3. 各トランザクションが再度レコード数をカウント（ファントム読み取りが発生）
	4. SSIが範囲クエリの競合を検知して一方のトランザクションをシリアライゼーション失敗で中断
*/

type PhantomRead struct {
	PatternBase
}

func NewPhantomRead(connectionString string) *PhantomRead {
	return &PhantomRead{
		PatternBase: NewPatternBase(connectionString, "PhantomRead"),
	}
}

func (p *PhantomRead) Looser(tx *sql.Tx, done chan struct{}) error {
	log.Println("Looser: Starting transaction...")

	// 最初の範囲クエリ、読み取りセットを確立
	var count1 int
	row := tx.QueryRow("SELECT COUNT(*) FROM test WHERE name >= 'A' AND name <= 'M'")
	if err := row.Scan(&count1); err != nil {
		log.Printf("**** Looser: Failed to read count1: %v", err)
		done <- struct{}{}
		return err
	}
	log.Printf("Looser: Count A-M = %d", count1)

	// 準備完了を通知して待機
	log.Println("Looser: About to send ready signal...")
	p.looserReady <- struct{}{}
	log.Println("Looser: About to wait for proceed signal...")
	<-p.proceedSignal

	// Winnerがクエリする範囲に挿入
	_, err := tx.Exec("INSERT INTO test (name) VALUES ('Charlie')")
	if err != nil {
		log.Printf("**** Looser: Insert failed: %v", err)
		done <- struct{}{}
		return err
	}
	log.Println("Looser: Inserted Charlie successfully")

	// 二回目の範囲クエリ、ファントムが見えるはず
	var count2 int
	row = tx.QueryRow("SELECT COUNT(*) FROM test WHERE name >= 'A' AND name <= 'M'")
	if err := row.Scan(&count2); err != nil {
		log.Printf("**** Looser: Failed to read count2: %v", err)
		done <- struct{}{}
		return err
	}
	log.Printf("Looser: New count A-M = %d", count2)

	// 書き込み完了を通知してコミット待機
	log.Println("Looser: About to send write done signal...")
	p.looserWriteDone <- struct{}{}
	log.Println("Looser: About to wait for commit signal...")
	<-p.commitSignal

	log.Println("Looser: Attempting to commit...")
	log.Println("Looser: About to send done signal...")
	done <- struct{}{}
	return nil
}

func (p *PhantomRead) Winner(tx *sql.Tx, done chan struct{}) error {
	log.Println("Winner: Starting transaction...")

	// 同じ範囲クエリ、ファントム読み取り依存関係を作成
	var count1 int
	row := tx.QueryRow("SELECT COUNT(*) FROM test WHERE name >= 'A' AND name <= 'M'")
	if err := row.Scan(&count1); err != nil {
		log.Printf("**** Winner: Failed to read count1: %v", err)
		done <- struct{}{}
		return err
	}
	log.Printf("Winner: Count A-M = %d", count1)

	// 準備完了を通知して待機
	log.Println("Winner: About to send ready signal...")
	p.winnerReady <- struct{}{}
	log.Println("Winner: About to wait for proceed signal...")
	<-p.proceedSignal

	// 同じ範囲に挿入、ファントム競合を作成
	_, err := tx.Exec("INSERT INTO test (name) VALUES ('Diana')")
	if err != nil {
		log.Printf("**** Winner: Insert failed: %v", err)
		done <- struct{}{}
		return err
	}
	log.Println("Winner: Inserted Diana successfully")

	// 二回目の範囲クエリ、別のファントムが見えるはず
	var count2 int
	row = tx.QueryRow("SELECT COUNT(*) FROM test WHERE name >= 'A' AND name <= 'M'")
	if err := row.Scan(&count2); err != nil {
		log.Printf("**** Winner: Failed to read count2: %v", err)
		done <- struct{}{}
		return err
	}
	log.Printf("Winner: New count A-M = %d", count2)

	// 書き込み完了を通知してコミット待機
	log.Println("Winner: About to send write done signal...")
	p.winnerWriteDone <- struct{}{}
	log.Println("Winner: About to wait for commit signal...")
	<-p.commitSignal

	log.Println("Winner: Attempting to commit...")
	log.Println("Winner: About to send done signal...")
	done <- struct{}{}
	return nil
}

func (p *PhantomRead) Do(db *sql.DB, done chan struct{}) {
	p.PatternBase.Do(db, done, p.Looser, p.Winner)
}
