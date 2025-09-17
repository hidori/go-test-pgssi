package pattern

import (
	"database/sql"
	"log"
)

/*
	読み取り-書き込み依存関係による競合（AliceとBobの相互更新による依存サイクルをテスト）

	SSIが検出すべき危険な依存サイクルを作成：
	1. LooserとWinnerの両方が同じデータセット（AliceとBobのレコード）を読み取り
	2. LooserはBobの値に基づいてAliceを更新、WinnerはAliceの値に基づいてBobを更新
	3. これにより一貫してシリアライズできない読み取り-書き込み依存サイクルが作成される
	4. SSIはこのパターンを検出して、一方のトランザクションをシリアライゼーション失敗で中断する
*/

type DirtyRead struct {
	PatternBase
}

func NewDirtyRead(connectionString string) *DirtyRead {
	return &DirtyRead{
		PatternBase: NewPatternBase(connectionString, "DirtyRead"),
	}
}

func (d *DirtyRead) Looser(tx *sql.Tx, done chan struct{}) error {
	log.Println("Looser: Starting serializable transaction...")

	// AliceとBobを読み取り、読み取り依存関係を確立
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
	log.Printf("Looser: Read Alice = %s, Bob = %s", aliceName, bobName)

	// 準備完了を通知して待機
	log.Println("Looser: About to send ready signal...")
	d.looserReady <- struct{}{}
	log.Println("Looser: About to wait for proceed signal...")
	<-d.proceedSignal

	// Bobの現在状態に基づいてAliceを更新、書き込み依存関係を作成
	log.Println("Looser: Attempting to update Alice...")
	_, err = tx.Exec("UPDATE test SET name = 'Alice_saw_' || $1 WHERE name = 'Alice'", bobName)
	if err != nil {
		log.Printf("**** Looser: Update failed: %v", err)
		done <- struct{}{}
		return err
	}
	log.Printf("Looser: Updated Alice to Alice_saw_%s", bobName)

	// 書き込み完了を通知してコミット待機
	log.Println("Looser: About to send write done signal...")
	d.looserWriteDone <- struct{}{}
	log.Println("Looser: About to wait for commit signal...")
	<-d.commitSignal

	log.Println("Looser: Attempting to commit...")
	log.Println("Looser: About to send done signal...")
	done <- struct{}{}
	return nil
}

func (d *DirtyRead) Winner(tx *sql.Tx, done chan struct{}) error {
	log.Println("Winner: Starting serializable transaction...")

	// AliceとBobを読み取り、読み取り依存関係を確立（Looserと同じデータ）
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
	log.Printf("Winner: Read Alice = %s, Bob = %s", aliceName, bobName)

	// 準備完了を通知して待機
	log.Println("Winner: About to send ready signal...")
	d.winnerReady <- struct{}{}
	log.Println("Winner: About to wait for proceed signal...")
	<-d.proceedSignal

	// Aliceの現在状態に基づいてBobを更新、書き込み依存関係を作成
	log.Println("Winner: Attempting to update Bob...")
	_, err = tx.Exec("UPDATE test SET name = 'Bob_saw_' || $1 WHERE name = 'Bob'", aliceName)
	if err != nil {
		log.Printf("**** Winner: Update failed: %v", err)
		done <- struct{}{}
		return err
	}
	log.Printf("Winner: Updated Bob to Bob_saw_%s", aliceName)

	// 書き込み完了を通知してコミット待機
	log.Println("Winner: About to send write done signal...")
	d.winnerWriteDone <- struct{}{}
	log.Println("Winner: About to wait for commit signal...")
	<-d.commitSignal

	log.Println("Winner: Attempting to commit...")
	log.Println("Winner: About to send done signal...")
	done <- struct{}{}
	return nil
}

func (d *DirtyRead) Do(db *sql.DB, done chan struct{}) {
	d.PatternBase.Do(db, done, d.Looser, d.Winner)
}
