package pattern

import (
	"database/sql"
	"log"
)

/*
	同じデータを読み取り、異なるレコードに書き込むことで発生する競合

	nameカラムを使用してライトスキューシナリオを作成：
	1. LooserとWinnerの両方がAliceとBobの名前を読み取り（読み取り依存関係を確立）
	2. Looserは"Alice_saw_Bob"にAliceを更新、Winnerは"Bob_saw_Alice"にBobを更新
	3. これによりライトスキューが発生：両トランザクションが同じデータを読み取るが異なるレコードに書き込む
	4. 更新は初期読み取り値に基づいており、読み取り-書き込み依存関係を作成
	5. SSIはこの依存パターンを検出して一方のトランザクションを中断すべき
*/

type WriteSkew struct {
	PatternBase
}

func NewWriteSkew(connectionString string) *WriteSkew {
	return &WriteSkew{
		PatternBase: NewPatternBase(connectionString, "WriteSkew"),
	}
}

func (w *WriteSkew) Looser(tx *sql.Tx, done chan struct{}) error {
	log.Println("Looser: Starting transaction...")

	// AliceとBobの名前を読み取り、読み取り依存関係を確立
	var aliceName, bobName string
	err := tx.QueryRow("SELECT name FROM test WHERE name = 'Alice'").Scan(&aliceName)
	if err != nil {
		log.Printf("**** Looser: Failed to read Alice: %v", err)
		done <- struct{}{}
		return err
	}
	err = tx.QueryRow("SELECT name FROM test WHERE name = 'Bob'").Scan(&bobName)
	if err != nil {
		log.Printf("**** Looser: Failed to read Bob: %v", err)
		done <- struct{}{}
		return err
	}

	log.Printf("Looser: Read Alice=%s, Bob=%s", aliceName, bobName)

	// 準備完了を通知して待機
	log.Println("Looser: About to send ready signal...")
	w.looserReady <- struct{}{}
	log.Println("Looser: About to wait for proceed signal...")
	<-w.proceedSignal

	// Bobから読み取った値に基づいてAliceを更新、書き込み依存関係を作成
	newAliceName := "Alice_saw_" + bobName
	_, err = tx.Exec("UPDATE test SET name = $1 WHERE name = 'Alice'", newAliceName)
	if err != nil {
		log.Printf("**** Looser: Update failed: %v", err)
		done <- struct{}{}
		return err
	}
	log.Printf("Looser: Updated Alice to %s", newAliceName)

	// 書き込み完了を通知してコミット待機
	log.Println("Looser: About to send write done signal...")
	w.looserWriteDone <- struct{}{}
	log.Println("Looser: About to wait for commit signal...")
	<-w.commitSignal

	log.Println("Looser: Attempting to commit...")
	log.Println("Looser: About to send done signal...")
	done <- struct{}{}
	return nil
}

func (w *WriteSkew) Winner(tx *sql.Tx, done chan struct{}) error {
	log.Println("Winner: Starting transaction...")

	// AliceとBobの名前を読み取り、読み取り依存関係を確立（Looserと同じパターン）
	var aliceName, bobName string
	err := tx.QueryRow("SELECT name FROM test WHERE name = 'Alice'").Scan(&aliceName)
	if err != nil {
		log.Printf("**** Winner: Failed to read Alice: %v", err)
		done <- struct{}{}
		return err
	}
	err = tx.QueryRow("SELECT name FROM test WHERE name = 'Bob'").Scan(&bobName)
	if err != nil {
		log.Printf("**** Winner: Failed to read Bob: %v", err)
		done <- struct{}{}
		return err
	}

	log.Printf("Winner: Read Alice=%s, Bob=%s", aliceName, bobName)

	// 準備完了を通知して待機
	log.Println("Winner: About to send ready signal...")
	w.winnerReady <- struct{}{}
	log.Println("Winner: About to wait for proceed signal...")
	<-w.proceedSignal

	// Aliceから読み取った値に基づいてBobを更新、Looserとのwrite skewを作成
	newBobName := "Bob_saw_" + aliceName
	_, err = tx.Exec("UPDATE test SET name = $1 WHERE name = 'Bob'", newBobName)
	if err != nil {
		log.Printf("**** Winner: Update failed: %v", err)
		done <- struct{}{}
		return err
	}
	log.Printf("Winner: Updated Bob to %s", newBobName)

	// 書き込み完了を通知してコミット待機
	log.Println("Winner: About to send write done signal...")
	w.winnerWriteDone <- struct{}{}
	log.Println("Winner: About to wait for commit signal...")
	<-w.commitSignal

	log.Println("Winner: Attempting to commit...")
	log.Println("Winner: About to send done signal...")
	done <- struct{}{}
	return nil
}

func (w *WriteSkew) Do(db *sql.DB, done chan struct{}) {
	w.PatternBase.Do(db, done, w.Looser, w.Winner)
}
