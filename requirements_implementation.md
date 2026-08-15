# Change Execution Control Plane 要件定義・実装方針

> バージョン: v1.0  
> 改訂日: 2026-08-13  
> 旧称: AI Agent Execution Harness  
> 想定規模: 1組織あたり50〜500サービス、初期参照規模200サービス

## 1. 文書の目的

本文書は、人間、CI、CLI、AI Agent から要求される本番変更を、安全に計画・実行・検証・停止・監査する **Change Execution Control Plane** の要件と実装方針を定義する。

最初のユースケースは、複数マイクロサービスからなる定期リリースの自動進行である。ただし、特定企業のService Console、特定クラウド、特定CI/CDを再実装しない。共通Core、宣言的Service Contract、Release Profile、型付きAdapterによって、異なる開発現場へ展開可能な製品として設計する。

AI Agentは自然言語の意図理解、Read情報の収集、説明、要約を担当する。認可、Policy、順序制御、実行許可、検証結果、Rollback条件は決定論的なシステムが判断する。

設計原則は次の一文で表す。

> **LLMをPolicy Engineにも、Credential Holderにも、Workflow Stateの正本にもしない。**
　
## 2. 主要な設計決定

| 領域 | 決定 |
|---|---|
| 製品境界 | AI専用Harnessではなく、全変更要求に共通のExecution Control Plane |
| 配置 | Managed SaaS Control Plane + 顧客環境Runner |
| Credential | Production Credentialは顧客環境から出さない |
| OSS | Runner、Contract、Adapter SDK、基本Planner、安全不変条件 |
| SaaS | Fleet管理、Workflow、Approval、Policy配布、監査、分析、Enterprise機能 |
| Workflow | Temporalを標準Durable Execution Engineとして採用 |
| Policy | OPA/Regoを標準とし、顧客環境Runnerで最終再評価 |
| サービス差分 | 個別WorkflowではなくService ContractとRelease Profileで表現 |
| 外部操作 | 任意shellではなく型付き高水準Actionのみ |
| 計画 | PlanとExecuteを分離し、hash、version、expiryで固定 |
| 状態 | Agent会話ではなくDurable Workflowと永続記録に保持 |
| 健全性 | 不明・欠損を成功扱いしない |
| 導入 | Shadow → Dry-run → Staging → 低リスクProduction → Criticalの順 |

## 3. Product Vision

```text
User / CI / CLI / AI Agent
             │ structured intent
             ▼
┌──────────────────────────────────────────────┐
│ Managed Control Plane                        │
│                                              │
│ Intent API ─→ Context ─→ Planner             │
│                    │         │                │
│ Identity ─→ Authorization ─→ Policy          │
│                              │               │
│ Approval ─→ Durable Workflow ─→ Audit        │
│                              │               │
│                    Signed Action Grant       │
└──────────────────────────────┬───────────────┘
                               │ outbound channel
                               ▼
┌──────────────────────────────────────────────┐
│ Customer Environment Runner                  │
│                                              │
│ Grant検証 → Local Policy → Credential Broker │
│                       │                      │
│               Typed Adapter                  │
│                  │       │                   │
│               Execute   Verify               │
└──────────────────┬────────┬──────────────────┘
                   │        │
              GitHub/AWS   Datadog/Prometheus
```

目標は、SaaS側が侵害、誤設定、誤推論されても、Runnerが許可されていない低水準操作を実行できない構造にすることである。

## 4. スコープ

### 4.1 初期スコープ

- 50〜200サービスのService Contract取込
- 変更対象サービスの特定
- 型付き依存グラフと循環・矛盾検出
- Release Waveと実行Plan生成
- UserとAgent Delegationの認可
- `ALLOW / DENY / REQUIRE_APPROVAL` Policy
- 上限付き並列実行
- Deploy、Status、Verify、Rollback
- Approval、Expire、Revoke、再承認
- 停止、再開、再試行、Worker障害復元
- 全Decision、Evidence、Actionの監査
- Web UI、REST API、CLI、MCP facade
- GitHub Actions、AWS ECS、Datadogの初期Adapter

### 4.2 後続スコープ

- Kubernetes / Argo CD / Flux
- Prometheus / Grafana / New Relic
- Feature Flag
- Terraform / Infrastructure Change
- Incident Responseの限定Action
- Cross-region / Multi-cluster Release
- Self-hosted Enterprise

### 4.3 対象外

- 任意shell実行
- 任意HTTP request
- 汎用AWS、Kubernetes、GitHub API proxy
- LLMによるProduction Policyの自動変更
- 未定義Actionの自律生成
- 不可逆DB Migrationの自動承認
- 完全自律の未知障害復旧
- Service Catalog、CI/CD、Observability製品の置き換え
- Break-glass CredentialのAgent利用

## 5. Deployment Model

### 5.1 Community OSS

単一組織が自環境で利用できる。Runner、CLI、Contract validator、基本Planner、Adapter SDK、基本UIまたはAPI、Mock環境を含む。

### 5.2 Managed SaaS + Customer Runner

主力形態。Control PlaneはSaaSで運用し、Runnerは顧客VPCまたはClusterで動作する。

- RunnerからOutbound HTTPS / mTLSで接続
- 顧客ネットワークへのInbound接続不要
- Production CredentialはRunner内で短寿命取得
- Raw logやSecretはSaaSへ送らない
- SaaSには正規化したEvidenceと参照URLを返す
- TenantごとにData RegionとRetentionを選択可能にする

### 5.3 Enterprise Self-hosted

Control Plane、Workflow Engine、Database、Policy配布を顧客環境へ配置する。同じContract、API、Runner、Adapterを使用し、別製品へForkしない。

## 6. Trust Boundaryと脅威前提

次を信頼しすぎない。

- LLM出力
- Agentが作ったTool引数
- 古いService Catalog情報
- 外部Webhookの重複・順序
- CI/CD APIのTimeout応答
- SaaSから送信されたAction要求
- 監視データの欠損
- 再試行による二重実行

RunnerがWriteを実行するための必要条件は次の積集合である。

```text
Valid Write
  = Valid customer identity proof
  ∩ Valid delegated agent scope
  ∩ Valid signed Action Grant
  ∩ Pinned plan hash
  ∩ Local mandatory policy ALLOW
  ∩ Fresh evidence and context
  ∩ Runner allowlist capability
  ∩ Valid approval proof when required
  ∩ Current state permits transition
```

SaaS署名だけでProduction Writeを許可しない。顧客IdP由来の主体情報、Runner側Policy、対象allowlist、期限、nonceを必須にする。

## 7. Actorと責務

| Actor | 責務 |
|---|---|
| Requester | 変更要求と業務情報の提供 |
| AI Agent | 意図の構造化、調査、説明、要約、限定Tool呼出し |
| Control Plane | Context、Plan、認可、Policy、Workflow、Approval、Audit |
| Customer Runner | 最終強制、Credential取得、Adapter実行、Evidence生成 |
| Service Owner | サービス固有の高リスク判断 |
| SRE | 横断リスク、回復不能状態、例外判断 |
| Security / Compliance | 強制Policy、監査要件、Retention |
| Platform Admin | Contract schema、Profile、Adapter、Runner管理 |

UserとAgentは別Subjectとして記録する。Agent chainがある場合は、委任元を切れ目なく記録する。

## 8. Service Contract

200サービスを個別実装しないため、各サービスはGitで管理されるService Contractを宣言する。

```yaml
apiVersion: execution.example.io/v1alpha1
kind: ServiceContract

metadata:
  name: payment-api
  owner: payment-team
  repository: github.com/example/monorepo
  riskTier: critical
  dataClassification: confidential

environments:
  production:
    releaseProfile: ecs-canary-critical
    runnerGroup: prod-ap-northeast-1
    concurrencyGroups:
      - payment-production
      - shared-payment-db

dependencies:
  - service: auth-api
    type: rollout-order
    condition: healthy
  - service: payment-schema
    type: schema-compatibility
    constraint: backward-compatible
  - resource: payment-primary-db
    type: shared-failure-domain

changeDetection:
  paths:
    - services/payment-api/**
  schemaPaths:
    - proto/payment/**
  migrationPaths:
    - db/payment/migrations/**

verification:
  profile: critical-http-api
  observationWindow: 10m
  checks:
    - type: error-rate
      threshold: "< 1%"
    - type: p95-latency
      threshold: "< 400ms"
    - type: synthetic
      check: payment-smoke

rollback:
  capability: automatic
  strategy: previous-task-definition
  deadline: 30m

approval:
  destructiveMigration:
    requiredRoles: [service-owner, sre]
    separationOfDuties: true
```

### 8.1 Contract要件

- JSON Schemaで検証可能
- `apiVersion`を必須とする
- Git commit SHAとcontent hashを保存する
- Owner、Risk Tier、Environment、Profileを必須とする
- Secretを含めない
- 参照するProfileとAdapter capabilityの存在を検証する
- 循環、未解決参照、矛盾をCIで拒否する
- Production利用前にContract readiness checkを通す

### 8.2 Source of Truth

- 宣言情報の正本: Git上のService Contract
- 補助情報: Service Console / Backstage / CMDB
- 動的状態の正本: 対象RuntimeとCI/CD
- 実行Workflow状態: Temporal
- 監査と入力snapshot: append-only Audit Store
- UI検索用状態: PostgreSQL read model

複数の正本がある領域を明示し、同期データを無条件に正本とみなさない。

## 9. Release ProfileとCapability Model

サービスは個別Workflowを持たず、少数のProfileを選択する。

初期Profile例:

- `ecs-rolling-standard`
- `ecs-canary-critical`
- `kubernetes-progressive`
- `worker-drain-and-replace`
- `schema-first-compatible`
- `manual-compensation-required`

Profileは次を定義する。

- 必須Precondition
- Deploy Action
- Verification sequence
- Observation window
- Concurrency constraints
- Rollback / Compensation
- Approval trigger
- TimeoutとRetry分類

AdapterはCapabilityを宣言する。

```text
deploy
get_status
cancel
rollback
verify_runtime
verify_metric
drain
shift_traffic
```

PlannerはContractが要求するCapabilityをRunnerとAdapterが満たすか、Plan生成時に検証する。未対応Capabilityを実行時まで持ち越さない。

## 10. Context IngestionとFreshness

Contextは次の経路で取得する。

- Git webhook / periodic reconciliation
- Service Catalog API
- CI/CD webhook and polling
- Runtime event
- Observability query
- Manual evidence with approver identity

各Contextに次を付与する。

- source
- source revision / external ID
- observed_at
- ingested_at
- schema version
- integrity hash
- freshness class

PlanはContext snapshotを参照し、実行前および各Step前に差分を確認する。差分が安全性に影響する場合はPlanを無効化し、再計画・再承認する。

## 11. Intent API

Control Planeは自然言語を直接実行しない。AgentまたはUIが構造化Intentへ変換する。

```json
{
  "intent_type": "release",
  "release_ref": "release-2026-08-15",
  "environment": "production",
  "requested_by": "user-123",
  "delegated_agent": "agent-456",
  "delegation_token_ref": "dlg-789",
  "service_selector": {
    "changed_since": "release-2026-08-08"
  },
  "mode": "plan"
}
```

曖昧なフィールド、未解決selector、存在しないEnvironmentは実行へ進めない。

## 12. Release Planner

Plannerは次の順序で決定論的にPlanを生成する。

1. Intentと主体を検証
2. 変更対象をSource revisionから確定
3. ContractとProfileを固定
4. 型付き依存グラフを構築
5. 循環、矛盾、未解決参照を検出
6. Schema compatibility、Migration分類を実行
7. Riskを計算
8. Policyを評価
9. Failure domainとConcurrency BudgetからWaveを構成
10. Precondition、Deploy、Verify、Compensation Stepを生成
11. Canonical JSON化しPlan hashを計算
12. expiryを設定してDry-run結果を返す

### 12.1 依存タイプの扱い

| Dependency type | Plannerの代表動作 |
|---|---|
| rollout-order | 前段StepのVerification成功まで待つ |
| schema-compatibility | Compatibility check成功を条件にする |
| runtime | 健全性確認に含めるが必ずしも順序化しない |
| shared-failure-domain | 同時実行を制限する |
| data-migration | Expand/Contract phaseを生成する |
| traffic | Traffic shiftと観測Stepを生成する |

### 12.2 Planの不変性

Execute要求には`plan_id`、`plan_hash`、`expires_at`を含める。次の変更でPlanを失効させる。

- 対象version変更
- ContractまたはProfile変更
- 依存関係変更
- Policyの安全性に関わる変更
- Approval対象入力変更
- Runner capability変更
- 有効期限切れ

## 13. Risk Model

RiskはLLMの主観評価だけで決めない。構造化された要素から決定論的に計算する。

```text
Risk inputs:
  service risk tier
  environment
  change type
  blast radius
  dependency fan-out
  rollback capability
  migration reversibility
  test and evidence completeness
  recent service health
  release window
```

LLMは変更内容から候補ラベルを提案できるが、Code diff scanner、path rule、schema checker等のEvidenceで確定する。確定不能なら高い側へ倒す。

## 14. Policy Architecture

Policyは階層化する。

```text
Platform Mandatory Policy
       ↓ 制約を弱められない
Environment Policy
       ↓
Team Policy
       ↓
Service Policy
       ↓
Change-specific facts
```

判定は次の三値を基本とする。

```text
ALLOW
DENY
REQUIRE_APPROVAL(required_roles, approval_policy)
```

必要に応じて`ALLOW_WITH_CONSTRAINTS`を内部表現として持ち、最大並列数、Canary比率、観測時間等をPlanへ反映する。ただし外部APIの判定理由は単純に保つ。

### 14.1 二重評価

1. Control PlaneがPlan生成と説明のために評価
2. RunnerがWrite直前に同じCanonical InputとPinned Policy Bundleで再評価

Runner側判定を最終的なPolicy Enforcement Pointとする。不一致時は実行せず、`POLICY_MISMATCH`として停止する。

### 14.2 Policy Bundle

- version、hash、署名を持つ
- TenantがProduction適用versionをpinできる
- Shadow、Canary、Enforceの配布段階を持つ
- Unit testとScenario testを必須にする
- Mandatory baselineをRunnerに同梱する
- Control Plane停止中も現在bundleを検証できる

## 15. AuthorizationとDelegation

### 15.1 Subject

- Human user
- Service account / CI
- AI Agent instance
- Runner identity
- Approver

### 15.2 Delegation

Agentへの委任は次を含む短寿命tokenとして表現する。

- delegated_by
- agent_id / agent_instance_id
- action scopes
- service selector
- environment
- maximum risk tier
- maximum affected services
- expiry
- audience
- correlation ID

Userの権限を超える委任は発行できない。AgentはUserの長期Credentialを保持しない。Delegation chain全体を監査する。

### 15.3 Separation of Duties

- RequesterとApproverの自己承認を禁止可能
- Service OwnerとSREの二者承認を表現可能
- Policy変更者とProduction適用者を分離可能
- Break-glassはAgent、SaaS、通常Runnerから利用不可

## 16. Signed Action Grant

Control PlaneからRunnerへ低水準命令を送らず、単一Stepに限定されたAction Grantを送る。

```json
{
  "tenant_id": "tenant-1",
  "run_id": "run-123",
  "step_id": "step-456",
  "actor": {
    "user_id": "user-123",
    "agent_id": "agent-456",
    "delegation_ref": "dlg-789"
  },
  "target": {
    "service": "payment-api",
    "environment": "production"
  },
  "action": {
    "type": "deploy",
    "artifact_digest": "sha256:..."
  },
  "plan_hash": "sha256:...",
  "contract_hash": "sha256:...",
  "policy_bundle_hash": "sha256:...",
  "evidence_hash": "sha256:...",
  "approval_proofs": [],
  "idempotency_key": "run-123:step-456:deploy:v1",
  "nonce": "...",
  "expires_at": "2026-08-15T10:05:00Z"
}
```

Action GrantはControl Plane署名を持つ。Runnerは署名、audience、期限、nonce、Plan、Contract、Policy、主体証明、Approval、ローカルallowlistを検証する。

## 17. Durable Workflow

長時間のApproval、観測、Retry、Rollback、Runner切断があるため、標準Workflow EngineとしてTemporalを採用する。

### 17.1 状態の責務

- Temporal History: 実行中Workflowの正本
- PostgreSQL: UI/API向けRead Model、Contract index、Approval index
- Audit Store: 不変のDecision、Evidence、Action記録
- External system: 実際に配置されたversionの正本

Projectionは再構築可能にする。UI用DBだけから外部Writeを判断しない。

### 17.2 ReleaseRun状態

```text
DRAFT
  ↓
PLANNING
  ├─ invalid ───────────────→ BLOCKED
  ↓
READY
  ├─ approval required ─────→ WAITING_APPROVAL
  └─ execute ───────────────→ RUNNING
                                 ├─ pause → PAUSED
                                 ├─ cancel → CANCELLING
                                 ├─ failure → ROLLING_BACK
                                 └─ complete → SUCCEEDED

ROLLING_BACK
  ├─ safe state reached → ROLLED_BACK
  └─ compensation failed → ESCALATED
```

### 17.3 Step状態

```text
PENDING → ELIGIBLE → GRANTING → DISPATCHED → EXECUTING
                                      ↓
                                  VERIFYING
                                  ├─ SUCCEEDED
                                  ├─ RETRY_WAIT
                                  ├─ ROLLING_BACK
                                  └─ ESCALATED
```

状態遷移はDomain層で検証し、全commandに期待state versionを持たせる。

## 18. Concurrencyと200サービス対応

200という数自体は大きなCompute負荷ではない。問題は、観測時間、外部API制限、共有failure domain、同時変更によるblast radiusである。

次の多層Budgetを適用する。

```text
Tenant production global budget
  ∩ Environment budget
  ∩ Region / Cluster budget
  ∩ Team budget
  ∩ Risk tier budget
  ∩ Shared resource budget
  ∩ Service + Environment exclusive lock
```

初期例:

| Scope | Initial budget example |
|---|---:|
| Production全体 | 10 |
| 同一Cluster | 3 |
| 同一Team | 2 |
| Critical Tier | 1 |
| 同一DB failure domain | 1 |
| 同一Service + Environment | 1 |

BudgetはPolicyから制約として返し、Schedulerが取得する。異常率が閾値を超えた場合はGlobal Circuit Breakerを開き、新規Stepを開始しない。

### 18.1 Release Wave

- Wave 0: Synthetic / preflight
- Wave 1: Low-risk canary services
- Wave 2: Independent standard services
- Wave 3: Fan-outの大きい基盤サービス
- Wave 4: Critical / irreversible-adjacent changes

実際の順序は依存グラフとPolicyから生成し、固定の番号だけに依存しない。

### 18.2 Backpressure

- Runner capacity広告
- Adapter rate limit
- Queue depth
- Verification query budget
- Tenant fairness
- Priority class

Control Planeは受け付けた全Stepを即時dispatchせず、lease付きでRunnerへ割り当てる。

## 19. Customer Runner

Runnerは顧客環境内の最終安全境界である。

### 19.1 必須機能

- mTLS / workload identity
- Action Grant署名検証
- nonce replay protection
- Local Policy評価
- Contract / Policy hash pinning
- Capability allowlist
- Credential Broker
- Adapter lifecycle
- Idempotency store
- Local execution journal
- Result / Evidence署名
- Heartbeat、capacity、version reporting
- Emergency freeze

### 19.2 切断時の動作

- 新規Production Writeは開始しない
- 実行中Deployは外部IDから状態確認する
- 事前許可されたCompensationはローカルに継続可能
- 判断不能なら安全な停止点でpauseする
- 接続復旧後にjournalをreconcileする
- Break-glassは別の人間運用経路を使用する

## 20. Adapter SDK

外部システムは必ず型付きAdapterを介する。

```go
type DeployAdapter interface {
    Capabilities(ctx context.Context) CapabilitySet
    Preflight(ctx context.Context, req PreflightRequest) (PreflightResult, error)
    Deploy(ctx context.Context, req DeployRequest) (DeploymentRef, error)
    GetStatus(ctx context.Context, ref DeploymentRef) (DeploymentStatus, error)
    Cancel(ctx context.Context, ref DeploymentRef) (CancelResult, error)
    Rollback(ctx context.Context, req RollbackRequest) (RollbackRef, error)
}

type VerificationAdapter interface {
    Collect(ctx context.Context, req EvidenceRequest) (Evidence, error)
    Evaluate(ctx context.Context, req VerificationRequest) (VerificationResult, error)
}
```

### 20.1 Adapter要件

- 許可フィールド以外を受け取らない
- 文字列の任意commandを受け取らない
- CredentialをControl PlaneやAgentから受け取らない
- 外部execution IDを永続化する
- Timeout時にblind retryしない
- retryable / terminal / unknownを分類する
- Rate limitとbackoffを実装する
- Dry-run、Mock、contract testを提供する
- Responseを正規化しEvidenceへ変換する
- Secret、token、raw sensitive payloadを返さない

## 21. Verification

デプロイ完了と健全性を別状態にする。

### 21.1 Verification Check

- Runtime desired/ready count
- Error rate
- Latency percentile
- Saturation
- Restart / crash loop
- Synthetic transaction
- Domain KPI
- Dependency health
- Log pattern
- Tracing-derived failure rate

### 21.2 判定

```text
PASS
FAIL
INCONCLUSIVE
MISSING
```

`INCONCLUSIVE`と`MISSING`をPASSへ変換しない。Policyに従い、観測延長、停止、Approval、Rollbackのいずれかへ進む。

### 21.3 Evidence

EvidenceにはQuery、観測期間、取得元、取得時刻、値、閾値、Adapter version、hashを含める。LLMによる要約はEvidence本体と分離する。

## 22. RollbackとCompensation

Actionごとに次を宣言する。

```text
automatic
approval-required
manual-only
unsupported
```

自動Rollbackの条件:

- 対象versionとprevious known-goodが確定
- Rollback capabilityがContractとAdapterの両方に存在
- Compensation deadline内
- PolicyがALLOW
- 追加の不可逆副作用が検出されていない

Rollback自体を独立Executionとして記録し、Rollback後にもVerificationを実行する。Rollback失敗時は全下流を停止し、最優先でEscalationする。

DB MigrationはExpand / Migrate / Contractを別Stepに分け、Contract phaseの自動実行は初期スコープ外とする。

## 23. Human Approval

Approvalは次を持つ。

- required roles
- quorum
- separation of duties
- reason code
- risk summary
- affected services
- Plan hash
- Policy version
- evidence snapshot
- expires_at
- approve / deny / revoke

次の場合はApprovalを失効させる。

- Plan hash変更
- 対象artifact変更
- Risk上昇
- Contract / Profile変更
- 必須Evidenceの陳腐化
- Approverの権限喪失
- 期限切れ

Slack等は通知・操作面として利用できるが、Approvalの正本はControl Planeに置く。リンクやボタンからの操作でもApprover identityを再認証する。

## 24. AuditとExplanation

監査では次の質問に回答できなければならない。

> 誰が、どのAgentへ、何を、どの範囲で委任し、どのContextとPolicyに基づき、誰の承認で、どのRunnerとAdapterが、何を実行し、何を観測して、なぜ次の状態へ進んだか。

### 24.1 Audit Event

- tenant_id
- correlation_id
- run_id / step_id
- actor_type / actor_id
- agent_id / delegated_by
- action
- target
- input hash
- plan / contract / policy version and hash
- decision / reason code
- approval proof reference
- external execution ID
- result and evidence hash
- timestamp
- Runner identity and version

Audit Storeは通常のApplication Logと分離し、実行系から変更・削除できない構成にする。RetentionとexportをTenant Policyで設定する。

### 24.2 Explanation

- System explanation: 構造化Reason CodeとEvidence
- AI explanation: 人間向け要約

両者を明確に区別し、AI要約を監査上の決定根拠にしない。

## 25. APIとTool Interface

### 25.1 REST API

```text
POST   /v1/plans
GET    /v1/plans/{id}
POST   /v1/release-runs
GET    /v1/release-runs/{id}
POST   /v1/release-runs/{id}:pause
POST   /v1/release-runs/{id}:resume
POST   /v1/release-runs/{id}:cancel
GET    /v1/release-runs/{id}/events

GET    /v1/approvals?status=pending
POST   /v1/approvals/{id}:approve
POST   /v1/approvals/{id}:deny
POST   /v1/approvals/{id}:revoke

GET    /v1/services
GET    /v1/services/{id}
GET    /v1/services/{id}/dependencies
POST   /v1/contracts:validate

GET    /v1/runners
GET    /v1/runners/{id}/capabilities
POST   /v1/runners/{id}:freeze
```

### 25.2 Agent Tool

```text
list_services
get_service_context
plan_release
explain_release_plan
start_release
get_release_status
pause_release
cancel_release
list_pending_approvals
get_incident_context
```

Agentへ`execute_shell`、`aws_api_call`、`kubectl`、任意HTTP、Policy変更Toolを公開しない。

## 26. Data Model

### 26.1 主要エンティティ

```text
Tenant
  id, region, isolation_mode, retention_policy

Service
  tenant_id, id, name, owner_team, risk_tier,
  contract_version, contract_hash, updated_at

Environment
  tenant_id, id, service_id, profile_id, runner_group

Dependency
  service_id, target_id, type, constraint, contract_version

ReleaseProfile
  id, version, required_capabilities, step_template, hash

Plan
  id, tenant_id, intent_id, status, canonical_input,
  plan_hash, context_snapshot_ref, expires_at

ReleaseRun
  id, tenant_id, plan_id, temporal_workflow_id,
  requested_by, delegated_agent_id, status, created_at

ReleaseStep
  id, run_id, service_id, action, wave, status,
  state_version, attempt, idempotency_key

PolicyDecision
  id, step_id, policy_version, policy_hash, decision,
  reason_code, canonical_input, input_hash, created_at

Approval
  id, step_id, required_roles, quorum, status,
  plan_hash, expires_at, decided_by, decided_at

ActionGrant
  id, step_id, runner_id, nonce, grant_hash,
  expires_at, dispatched_at, consumed_at

Execution
  id, step_id, runner_id, adapter, action,
  external_execution_id, status, started_at, finished_at

Evidence
  id, execution_id, source, check_type, query_hash,
  observed_value, threshold, status, observed_at, evidence_hash

AuditEvent
  tenant_id, id, correlation_id, actor, action,
  target, result, payload_ref, timestamp
```

### 26.2 一貫性

- `(tenant_id, adapter, idempotency_key)` unique
- `service + environment` exclusive lease
- optimistic state version
- canonical JSON + hash
- signed Action Grant nonce unique
- ApprovalはPlan hashに固定
- OutboxでDB更新とEvent発行を原子的にする
- Webhook event IDで重複排除

## 27. Multi-tenancy

- 全レコードにtenant_id
- Tenant境界をApplication層だけでなくDB Policyでも強制
- Tenant別encryption key
- Tenant別Policy Bundleとsigning trust
- Tenant別rate limitとWorkflow quota
- Cross-tenant queryを通常実行Roleから禁止
- EnterpriseではDedicated DB / Namespace / Control Planeを選択可能
- Operator accessを監査し、JIT accessを採用

Production Actionの署名鍵、Audit鍵、Tenant data encryption鍵を分離する。

## 28. Security Requirements

- OIDC / SAML SSO
- SCIM lifecycle
- Workload identity / mTLS
- Short-lived credentials
- Secret zero-storage in SaaS
- Egress allowlist
- Signed artifacts、contracts、policy bundles、grants
- Webhook signatureとreplay protection
- Artifact digest pinning
- Supply-chain provenance verification
- Encryption in transit and at rest
- Sensitive field redaction before audit ingestion
- Dependency、container、SAST、DAST scan
- Regular threat modeling and penetration test
- Runner auto-updateは署名検証と段階Rolloutを必須にする

## 29. 障害時の振る舞い

| 障害 | 振る舞い |
|---|---|
| Policy取得不能 | pinned bundleで検証、期限切れなら新規Write停止 |
| Policy不一致 | 実行せず`POLICY_MISMATCH` |
| Context stale | 再取得、不能ならPlan失効 |
| SaaS切断 | 新規Write停止、許可済みCompensationのみ継続可能 |
| Runner crash | lease expiry後にjournalと外部IDからreconcile |
| Temporal Worker crash | Workflow historyから再開 |
| Deploy API timeout | 外部ID・idempotency keyで照会しblind retryしない |
| Metrics missing | PASSにせず観測延長または停止 |
| Approval timeout | BlockまたはCancelし通知 |
| Audit永続化不能 | Action Grantを発行しない |
| Rollback failure | 下流停止、Global Budget縮小、Escalate |
| 異常率上昇 | Circuit Breakerを開き新規Step停止 |
| Duplicate webhook | event IDで無害化 |

## 30. Non-functional Requirements

| ID | 要件 | 初期目標 |
|---|---|---|
| NFR-01 | Fail Closed | Auth/Policy/Context不明時にWrite 0件 |
| NFR-02 | Idempotency | 同一keyの外部Write 1回以下 |
| NFR-03 | Durability | Process停止後のWorkflow復元100% |
| NFR-04 | Auditability | Production Writeの監査欠損0件 |
| NFR-05 | Concurrency Safety | 競合Write 0件 |
| NFR-06 | Plan Reproducibility | hashから入力versionを再現可能 |
| NFR-07 | Control Plane Availability | Managed Production 99.9%以上を初期目標 |
| NFR-08 | Runner Availability | 顧客要件に応じ冗長配置 |
| NFR-09 | Planning Performance | 500 services / 5,000 edgesを10秒以内目標 |
| NFR-10 | Dispatch Latency | eligibleからdispatch p95 5秒以内目標 |
| NFR-11 | Tenant Isolation | Cross-tenant access 0件 |
| NFR-12 | Observability | logs / metrics / traces / workflow link |

数値はPilot計測後に確定し、特にAvailabilityとPlanning Performanceを契約SLOと内部目標に分ける。

## 31. Observability

### Platform Metrics

- plan generation latency
- workflow / activity latency
- runner heartbeat lag
- queue depth
- grant rejection reason
- policy decision distribution
- adapter error classification
- verification missing rate
- rollback success rate
- circuit breaker state

### Product Metrics

- release lead time
- automated completion rate
- human sequential action count
- HITL precision
- approval latency
- mean time to safe state
- onboarding time per service
- services managed per Platform engineer

TenantのアプリケーションMetricと製品運用Metricを混同しない。

## 32. Testing Strategy

### 32.1 Unit Test

- 型付きグラフと循環検出
- Plan canonicalization
- Risk classification boundary
- Policy hierarchy
- 状態遷移
- Grant validation
- idempotency
- retry classification

### 32.2 Contract Test

- Contract schema version compatibility
- Profile required capabilities
- Adapter request/response
- Policy input/output
- Runner protocol
- MCP Tool schema

### 32.3 Integration Test

- Temporal replay / worker recovery
- PostgreSQL outbox / projection rebuild
- OPA bundle更新とrollback
- Approval失効
- Runner disconnect / reconnect
- External API timeout and reconciliation
- Multi-tenant isolation

### 32.4 Scenario Test

- 200 services all success
- DAG並列化とBudget制御
- Schema incompatibilityでPlan拒否
- CI失敗で開始拒否
- 依存Step失敗で下流停止
- Canary失敗でGlobal freeze
- Missing metricで停止
- 自動Rollback成功
- Rollback失敗でEscalation
- destructive migrationで二者承認
- Approval期限切れ
- Plan変更によるApproval失効
- Agent停止後もRun継続
- SaaS切断中の安全停止
- 重複start / webhook / grant replay
- Control PlaneとRunnerのPolicy不一致

### 32.5 Safety Invariants

1. 有効なPolicy ALLOWまたはApprovalなしにExecutionを作らない
2. Runnerの最終Policy ALLOWなしに外部Writeしない
3. 未成功の必須依存Stepを持つStepを実行しない
4. 同一idempotency keyで外部Writeを複数回行わない
5. Production WriteはUser/CI主体とAgent委任情報を持つ
6. Plan、Contract、Policy hashのないProduction Writeを許可しない
7. Expiredまたは再利用nonceのGrantを拒否する
8. Missing Evidenceを成功扱いしない
9. Rollback失敗後に下流Stepを開始しない
10. Tenant境界を跨ぐActionを実行しない

Property-based testとfault injectionでこれらを継続検証する。

## 33. 200サービスへの導入計画

一斉移行せず、サービス数ではなく特性の代表性とReadinessで段階導入する。

### Stage 0: Inventory

- Service、Owner、Environmentを棚卸し
- Deploy方式をクラスタリング
- 依存タイプを分類
- Rollback capabilityを確認
- 5〜10個のRelease Profile候補を作る

### Stage 1: Shadow

- 現行リリースからPlanを生成
- 実行せず、人間の順序・判断と比較
- Contract不足と誤った依存を収集

対象: 代表的な20サービス。

### Stage 2: Dry-run / Staging

- Mock Adapterから実Adapterへ移行
- StagingでDeploy、Verify、Rollback
- Failure injectionを実施

対象: StatelessかつRollback可能な20〜30サービス。

### Stage 3: Production Low Risk

- 5〜10サービスから開始
- 初期は自動Deployと停止、RollbackはApprovalでもよい
- Budgetを保守的に設定

### Stage 4: Expansion

- 30 → 80 → 150 → 200へWave拡大
- Profileで扱えない例外を分析
- 例外を新Profileにするか手動維持するか判断

### Stage 5: Critical

- Payment、Auth、Shared DB、基盤サービス
- Canary、長い観測、二者承認
- GameDayとRollback drill合格後に自動化

### Service Readiness Gate

- Contract valid
- Owner active
- Profile selected
- Adapter capability available
- Verification machine-evaluable
- Rollback testedまたはmanual-only明記
- Dependency reviewed
- Audit export verified
- Incident contact configured

## 34. 実装ロードマップ

### Phase 1: Local OSS Core

- Contract schema / validator
- typed dependency graph
- Planner / dry-run
- Mock Adapter
- OPA policy pack
- Temporal local environment
- CLIと最小UI

完了条件: 20サービスの正常系と主要失敗系を再現可能にする。

### Phase 2: Runner Safety Boundary

- signed Action Grant
- local policy reevaluation
- idempotency journal
- Credential Broker interface
- Evidence signing
- disconnect / reconnect

完了条件: Control Planeの要求だけでは任意Writeできない。

### Phase 3: Managed Control Plane

- tenant / identity / RBAC
- workflow fleet
- approval UI
- audit store
- policy bundle management
- runner management

完了条件: 3つの隔離Tenantでend-to-end実行できる。

### Phase 4: First Real Adapters

- GitHub Actions
- AWS ECS
- Datadog
- Slack notification / approval link

完了条件: Design PartnerのStagingで実行・検証・Rollbackできる。

### Phase 5: Production Pilot

- low-risk Production services
- circuit breaker
- SLO / on-call
- audit export
- security review

完了条件: 30日間、Policy bypassと二重実行0件で運用する。

### Phase 6: Scale and Enterprise

- 200-service planning and scheduling
- dedicated tenancy
- self-hosted packaging
- enterprise connectors
- regional deployment / DR

## 35. OSS / SaaS機能境界

| Capability | OSS | Managed SaaS | Enterprise Self-hosted |
|---|:---:|:---:|:---:|
| Contract / Adapter SDK | ✓ | ✓ | ✓ |
| Runner / local PEP | ✓ | ✓ | ✓ |
| Basic Planner / CLI | ✓ | ✓ | ✓ |
| Single-org local workflow | ✓ | ✓ | ✓ |
| Fleet management |  | ✓ | ✓ |
| SSO / SCIM / advanced RBAC |  | ✓ | ✓ |
| Managed Approval UI |  | ✓ | ✓ |
| Policy distribution / staged rollout |  | ✓ | ✓ |
| Long-term audit / analytics |  | ✓ | ✓ |
| SLA / 24x7 support |  | ✓ | ✓ |
| Dedicated deployment |  | option | ✓ |
| Air-gapped |  |  | ✓ |

安全不変条件を意図的に有料版だけへ閉じ込めない。商用価値は、集中管理、運用品質、Enterprise Identity、監査、分析、Supportに置く。

## 36. Demo Scenarios

### Demo A: 200サービスPlan

```text
Changed services: 63
Affected services: 91
Release waves: 8
Maximum planned concurrency: 10
Critical approvals: 3
Blocked contracts: 2
Estimated observation critical path: 47m
```

人間が見る前に、矛盾、未定義Rollback、古いContextをPlan段階で示す。

### Demo B: 正常系

```text
User: release-2026-08-15をproductionへリリースして

Plan hash: sha256:...
Services: 34
Policy: ALLOW_WITH_CONSTRAINTS

Result: 34/34 deployed and verified
Human sequential actions: 0
```

### Demo C: Canary Failure

```text
payment-api canary verification failed
error_rate: 8.3% / threshold: 1.0%

New production steps: frozen
Automatic rollback: succeeded
Rollback verification: passed
Escalation: payment-team + SRE
```

### Demo D: Agent Compromise Containment

```text
Requested action: arbitrary aws_api_call
Runner capability: not allowed
Grant validation: failed
External write: 0
Audit reason: UNSUPPORTED_ACTION
```

デモの訴求点はAIの推論能力ではなく、誤った要求でも越えられない境界と、失敗から安全状態へ戻る能力である。

## 37. 成功指標

### MVP

- Safety Invariants違反: 0
- 二重実行: 0
- Tenant境界違反: 0
- 20サービスScenario完遂率: 100%
- Worker / Runner復元Scenario: 100%
- Audit completeness: 100%

### Pilot

- 正常系自動完遂率
- 人間の逐次操作削減率
- Release lead time短縮
- Mean time to safe state
- False / missed escalation
- Rollback成功率
- Service onboarding時間
- Runner / Adapter運用負荷

数値目標は顧客の現状Baselineを計測してから合意する。

## 38. 未決事項

1. Temporal Cloud、SaaS内self-managed、Tenant dedicatedの選択基準
2. OSS Coreのライセンス
3. Action Grantの署名形式と顧客IdP proofの標準
4. Policy Bundleの配布形式と署名チェーン
5. Service Contractをmono-repoで集約するか各repoへ置くか
6. Audit payloadの地域・Retention要件
7. SaaS切断時に許可する事前承認済みCompensationの範囲
8. Slack Approvalと専用UIの責務境界
9. 最初の有料Design Partnerで対応するAdapter
10. Professional ServicesとProduct機能の境界

## 39. Design Partnerへの確認事項

- 直近の複数サービス変更はどのように進行したか
- どの判断が機械化できず、なぜ人間が必要か
- Service metadataと依存関係の正本は何か
- 主要なRelease Profileは何種類に分類できるか
- Rollback可能なサービスの割合はどの程度か
- 監視値を機械的な成功判定へ利用できるか
- Production Credentialを持つRunnerの配置条件は何か
- SaaSへ送信できないデータは何か
- ApprovalとAuditの既存基盤はあるか
- Agentへ最初に開放したいWrite Actionは何か
- 購入判断者、運用責任者、セキュリティ承認者は誰か
- 成功を時間、負荷、事故率、監査、Agent利用のどれで測るか

## 40. 最終アーキテクチャ原則

> **Open execution, managed control, customer-held credentials.**

> **Declare service differences; do not encode 200 bespoke workflows.**

> **Plan centrally, enforce locally, verify from evidence, and escalate only accountable decisions.**

## 41. 参考資料

- [LayerX: SRE NEXT 2026 登壇してきました 〜バクラク開発者プラットフォーム大公開〜](https://tech.layerx.co.jp/entry/2026/07/22/150006)
- [LayerX: B2B SaaS における AI Agent 向けの認可に向けた課題](https://tech.layerx.co.jp/entry/2025/09/25/213844)
- [DORA: Platform engineering](https://dora.dev/capabilities/platform-engineering/)
- [CNCF Annual Survey 2024](https://www.cncf.io/reports/cncf-annual-survey-2024/)
- [Open Policy Agent Documentation](https://www.openpolicyagent.org/docs/)
- [OPA Management APIs](https://www.openpolicyagent.org/docs/management-introduction)
- [Temporal Documentation](https://docs.temporal.io/)
- [Temporal Cloud Architecture](https://temporal.io/cloud)
- [Model Context Protocol Specification](https://modelcontextprotocol.io/specification/)
