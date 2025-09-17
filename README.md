# Go: Test for Serializable Snapshot Isolation (SSI)

## 概要

SSIは**predicate locks**（述語ロック）を使用して、並行実行されるトランザクション間の読み書き依存関係を追跡し、シリアライゼーション異常を検出する。重要な特徴は以下のとおり。

- **非ブロッキング**：predicate locksは実際のブロッキングを発生させない
- **依存関係追跡**：トランザクション間の読み書き依存関係を監視する
- **競合検出**：シリアル実行と矛盾する状態を検出時に、一方のトランザクションを強制ロールバックする
- **First Committer Wins**：競合検出時、先にコミットしたトランザクション（分離レベル問わず）が成功し、後続の`SERIALIZABLE`レベルのトランザクションのみが serialization failure でロールバックする（他の分離レベルのトランザクションは影響を受けない）

*参考*：[PostgreSQL公式ドキュメント - Transaction Isolation](https://www.postgresql.org/docs/current/transaction-iso.html#XACT-SERIALIZABLE)

## PostgreSQLでのSSI設定

### 基本設定

**個別トランザクションでの指定**：

   ```sql
   BEGIN TRANSACTION ISOLATION LEVEL SERIALIZABLE;
   -- または
   SET TRANSACTION ISOLATION LEVEL SERIALIZABLE;
   ```

### 推奨パラメータ

SSIを使用する環境では以下の設定調整を推奨：

- **`max_pred_locks_per_transaction`**（デフォルト: 64）：
  - predicate lockの最大数を制御
  - 複雑なクエリや大量データ処理では増加を検討

- **`max_pred_locks_per_relation`**（デフォルト: -2）：
  - リレーション毎のpredicate lock上限
  - `-2`は`max_pred_locks_per_transaction / 2`を意味

- **`max_pred_locks_per_page`**（デフォルト: 2）：
  - ページレベルのpredicate lock上限

### 注意事項

- SSIは**PostgreSQL 9.1以降**でのみ利用可能
- 古いバージョンの`SERIALIZABLE`は実際には`REPEATABLE READ`と同等
- レプリケーション環境では追加考慮が必要

*参考*：

- [PostgreSQL公式ドキュメント - Runtime Config](https://www.postgresql.org/docs/current/runtime-config-locks.html#RUNTIME-CONFIG-LOCKS-PREDICATE)
- [PostgreSQL公式ドキュメント - Transaction Isolation](https://www.postgresql.org/docs/current/transaction-iso.html#XACT-SERIALIZABLE)

## SSI競合検出の成立条件

SSIによる競合検出は以下の条件で動作：

1. **最低1つのトランザクションがSERIALIZABLEレベル**：
   - 競合検出を行うには、関与するトランザクションの少なくとも1つが `SERIALIZABLE` 分離レベルである必要がある
   - `SERIALIZABLE` トランザクションは、他のあらゆる分離レベルとの競合を検出可能
   - **重要**：競合検出時は **`SERIALIZABLE`レベルのトランザクションがfail** する（他の分離レベルのトランザクションは成功）

2. **読み書き依存関係の形成**：
   - トランザクションA が読み取った範囲に、トランザクションB が書き込みを行う
   - または、互いの読み取り範囲に対して書き込みを行う（write skew）

3. **Dangerous Structure の検出**：
   - 3つ以上のトランザクションでサイクル状の依存関係が形成される場合
   - 任意のシリアル実行順序と矛盾する状態が発生した場合

*参考*：[PostgreSQL Wiki - SSI](https://wiki.postgresql.org/wiki/SSI)

## 競合パターン

### SSIで検出可能な競合

- **Write Skew**：SSIの主要な検出対象
- **Phantom Read**：範囲クエリでの新規レコード挿入
- **Non-repeatable Read**：同一レコードの値変更

### SSIで検出されない競合

- **Dirty Read**：PostgreSQLの`READ COMMITTED`以上では元々発生しない
- **Lost Update**：PostgreSQLのrow-level lockingで自動的に防止される

## 実装されているテスト

### 実行方法

```bash
make compose/up
make run
```

### テーブル構造

以下のテーブル定義を使用：

```sql
CREATE TABLE test (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL
);
```

### データ初期化

各テストパターンの実行前に以下の処理を実行：

1. **全レコード削除**: `DELETE FROM test`
2. **基本データ投入**: `Alice` と `Bob` レコードを挿入

この初期化により、各テストが一貫した状態から開始されることを保証。

### テストパターン

このプロジェクトでは以下の競合パターンをテスト：

- Dirty Read（実際はDirty Write競合）
- Phantom Read
- Write Skew

#### Dirty Read

**概要**：読み取り-書き込み依存関係による競合（AliceとBobの相互更新による依存サイクルをテスト）

**シナリオ**：

SSIが検出すべき危険な依存サイクルを作成：

1. LooserとWinnerの両方が同じデータセット（AliceとBobのレコード）を読み取り
2. LooserはBobの値に基づいてAliceを更新、WinnerはAliceの値に基づいてBobを更新
3. これにより一貫してシリアライズできない読み取り-書き込み依存サイクルが作成される
4. SSIはこのパターンを検出して、一方のトランザクションをシリアライゼーション失敗で中断する

**SSI動作**：

- トランザクション間の読み取り-書き込み依存関係を追跡
- 依存サイクルの検出により競合を判定
- First Committer Winsルールで解決

#### Phantom Read

**概要**：範囲クエリで新規レコードが「幻」のように出現する競合

**シナリオ**：

SSIがファントムリード現象を検出するかテスト：

1. LooserとWinnerの両方がA-M範囲のレコード数をカウント（読み取りセット確立）
2. Looserが'Charlie'を挿入、Winnerが'Diana'を挿入（どちらもA-M範囲内）
3. 各トランザクションが再度レコード数をカウント（ファントム読み取りが発生）
4. SSIが範囲クエリの競合を検知して一方のトランザクションをシリアライゼーション失敗で中断

**SSI動作**：

- Predicate locksで範囲読み取りを追跡
- 範囲への挿入操作で競合検出

#### Write Skew

**概要**：同じデータを読み取り、異なるレコードに書き込むことで発生する競合

**シナリオ**：

nameカラムを使用してライトスキューシナリオを作成：

1. LooserとWinnerの両方がAliceとBobの名前を読み取り（読み取り依存関係を確立）
2. Looserは"Alice_saw_Bob"にAliceを更新、Winnerは"Bob_saw_Alice"にBobを更新
3. これによりライトスキューが発生：両トランザクションが同じデータを読み取るが異なるレコードに書き込む
4. 更新は初期読み取り値に基づいており、読み取り-書き込み依存関係を作成
5. SSIはこの依存パターンを検出して一方のトランザクションを中断すべき

**SSI動作**：

- 各トランザクションが読み取った範囲に対する書き込み依存関係を検出
- ビジネスルール違反を防止
