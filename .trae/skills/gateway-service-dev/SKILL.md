---
name: "kronos-service"
description: "Scaffolds Go gRPC microservice components (model/repo/service/migration/locale). Invoke when adding new business entities, CRUD APIs, or gRPC endpoints to an kronos-service project."
---

# kronos Service - Go gRPC Microservice Development

You are an expert in the kronos microservice architecture. When the user asks to add a new feature, entity, or API to an kronos-service project, follow these conventions precisely.

## Project Architecture

```
project/
├── bootstrap/          # Server init, DI, gRPC/HTTP registration
│   ├── server.go           # Gateway struct, Run(), lifecycle
│   ├── services.go         # initServices(), registerGRPCServices(), registerHTTPHandlers()
│   ├── database.go         # initDatabase(), migration trigger
│   └── subscriber.go       # Event subscribers (optional)
├── constants/          # Service name constants
├── errors/             # Business error codes + i18n error helpers
│   ├── codes.go            # BizErrCode* constants
│   ├── errors.go           # NewI18nError(), NewI18nErrorWithTemplate()
│   └── grpc.go             # ToGRPCError(), MapBizCodeToGRPCCode()
├── locales/            # i18n JSON files (zh.json, en.json, etc.)
├── migrations/         # Database auto-migration
│   ├── auto_migrate.go     # GetAllModels(), GetAllIndexes(), GetAllComments()
│   └── migrate.go          # SeedData()
├── middleware/          # gRPC interceptors
├── models/             # GORM models + base types
│   ├── base.go             # BuildPagination(), PBConvertible, LocalizedTextList
│   └── table_names.go      # TableName constants
├── repository/         # Data access layer
│   ├── factory.go          # RepositoryFactory singleton
│   └── *_repository.go     # Per-entity repositories
├── service/            # Business logic layer
│   ├── idgen.go            # NewSnowflakeID()
│   ├── *_service.go        # Main service impl (holds RepositoryFactory)
│   └── *_service.go        # Per-entity service methods
└── resources/          # YAML configs (gateway-{env}.yaml)
```

## Key Dependencies

| Package | Purpose |
|---------|---------|
| `github.com/kamalyes/go-rpc-gateway` | Gateway framework (gRPC + HTTP) |
| `github.com/kamalyes/go-sqlbuilder` | ORM base repository, migrator |
| `github.com/kamalyes/go-pbmo` | Proto ↔ Model bidirectional conversion |
| `github.com/kamalyes/go-config` | Environment config |
| `github.com/kamalyes/go-toolbox` | ID generation, utilities |
| `{org}/{prefix}-share-proto/pb/*` | Generated proto Go code |

## Adding a New Entity - Complete Checklist

When the user says "add a XXX entity/feature", perform ALL of the following steps:

### Step 1: Model (`models/xxx_model.go`)

```go
package models

import (
    "time"
    commonpb "{share-proto-module}/pb/common"
    "github.com/kamalyes/go-pbmo"
)

// XxxModel - field names must match proto for PBMO auto-conversion
type XxxModel struct {
    XxxId      string                    `gorm:"column:xxx_id;type:varchar(36);primaryKey;comment:XXX ID" json:"xxx_id"`
    Name       string                    `gorm:"column:name;type:varchar(128);not null;comment:名称" json:"name"`
    Status     int32                     `gorm:"column:status;type:tinyint;default:1;not null;comment:状态(1:正常,2:禁用)" json:"status"`
    I18nNames  models.LocalizedTextList  `gorm:"column:i18n_names;type:json;comment:多语言名称" json:"i18n_names"`
    CreatedAt  time.Time                 `gorm:"column:created_at;autoCreateTime;comment:创建时间" json:"created_at"`
    UpdatedAt  time.Time                 `gorm:"column:updated_at;autoUpdateTime;comment:更新时间" json:"updated_at"`
}

func (XxxModel) TableName() string { return "xxxs" }
func (XxxModel) TableComment() string { return "XXX表" }
```

**Rules:**
- Field names must match proto field names (PascalCase) for PBMO compatibility
- Use `varchar(36)` for string PKs (Snowflake short hash)
- Use `tinyint` for status/enums
- Use `models.LocalizedTextList` + `gorm:"type:json"` for i18n fields
- Always include `CreatedAt`/`UpdatedAt` with auto time tags

### Step 2: Table Name (`models/table_names.go`)

Add a new variable:
```go
var TableNameXxx = XxxModel{}.TableName()
```

### Step 3: Repository (`repository/xxx_repository.go`)

```go
package repository

import (
    "context"
    "{module}/models"
    sqlrepo "github.com/kamalyes/go-sqlbuilder/repository"
)

type XxxRepository struct {
    *sqlrepo.BaseRepository[models.XxxModel]
}

func NewXxxRepository() *XxxRepository {
    return &XxxRepository{
        BaseRepository: newBaseRepository[models.XxxModel](models.XxxModel{}.TableName()),
    }
}

func (r *XxxRepository) GetByID(ctx context.Context, id string) (*models.XxxModel, error) {
    return r.GetByFilter(ctx, sqlrepo.NewEqFilter("xxx_id", id))
}

func (r *XxxRepository) ListWithPagination(ctx context.Context, query *sqlrepo.Query, pagination *sqlrepo.Pagination32) ([]*models.XxxModel, *sqlrepo.Pagination32, error) {
    return r.ListWithPagination32(ctx, query, pagination)
}

func (r *XxxRepository) UpdateByID(ctx context.Context, id string, updates map[string]interface{}) error {
    return r.UpdateFieldsByFilters(ctx, updates, sqlrepo.NewEqFilter("xxx_id", id))
}

func (r *XxxRepository) DeleteByID(ctx context.Context, id string) error {
    return r.DeleteByFilters(ctx, sqlrepo.NewEqFilter("xxx_id", id))
}
```

**Available BaseRepository methods:** `Create`, `GetByFilter`, `Count`, `ListWithPagination32`, `UpdateFieldsByFilters`, `DeleteByFilters`, `ExistsByFilter`

### Step 4: Register Repository in Factory (`repository/factory.go`)

Add field to `RepositoryFactory` struct and initialize in `NewRepositoryFactory()`:
```go
type RepositoryFactory struct {
    Xxx *XxxRepository  // add this
}

factoryOnce.Do(func() {
    factoryInstance = &RepositoryFactory{
        Xxx: NewXxxRepository(),  // add this
    }
})
```

### Step 5: Service (`service/xxx_service.go`)

```go
package service

import (
    "context"
    "{module}/models"
    "{module}/repository"
    sqlrepo "github.com/kamalyes/go-sqlbuilder/repository"
)

type XxxService struct {
    repo *repository.XxxRepository
}

func NewXxxService(repo *repository.XxxRepository) *XxxService {
    return &XxxService{repo: repo}
}

func (s *XxxService) GetByID(ctx context.Context, id string) (*models.XxxModel, error) {
    return s.repo.GetByID(ctx, id)
}

func (s *XxxService) List(ctx context.Context, query *sqlrepo.Query, pagination *sqlrepo.Pagination32) ([]*models.XxxModel, *sqlrepo.Pagination32, error) {
    return s.repo.ListWithPagination(ctx, query, pagination)
}

func (s *XxxService) Create(ctx context.Context, model *models.XxxModel) error {
    _, err := s.repo.Create(ctx, model)
    return err
}

func (s *XxxService) Update(ctx context.Context, id string, updates map[string]interface{}) error {
    return s.repo.UpdateByID(ctx, id, updates)
}

func (s *XxxService) Delete(ctx context.Context, id string) error {
    return s.repo.DeleteByID(ctx, id)
}
```

### Step 6: Register Service in Main Service (`service/{service_name}_service.go`)

Add the sub-service field and initialize it:
```go
type XxxServiceImpl struct {
    repo *repository.RepositoryFactory
    xxx  *XxxService  // add this
}

func NewXxxService(repo *repository.RepositoryFactory) *XxxServiceImpl {
    return &XxxServiceImpl{
        repo: repo,
        xxx:  NewXxxService(repo.Xxx),  // add this
    }
}
```

### Step 7: Migration (`migrations/auto_migrate.go`)

Add to `GetAllModels()`:
```go
&models.XxxModel{},
```

Add to `GetAllIndexes()` if needed:
```go
{
    Table:   models.TableNameXxx,
    Name:    "idx_xxxs_name",
    Columns: []string{"name"},
    Unique:  true,
},
```

Add to `GetAllComments()`:
```go
{
    Table:   models.TableNameXxx,
    Comment: models.XxxModel{}.TableComment(),
},
```

### Step 8: Error Codes (`errors/codes.go`)

Add entity-specific error codes:
```go
BizErrCodeXxxNotFound      = "error.xxx_not_found"
BizErrCodeXxxNameExists    = "error.xxx_name_exists"
BizErrCodeXxxStatusInvalid = "error.xxx_status_invalid"
```

### Step 9: Locale Files (`locales/*.json`)

Add error messages to ALL locale files. Example for `zh.json`:
```json
"error.xxx_not_found": "XXX未找到",
"error.xxx_name_exists": "XXX名称已存在",
"error.xxx_status_invalid": "XXX状态无效"
```

Example for `en.json`:
```json
"error.xxx_not_found": "XXX not found",
"error.xxx_name_exists": "XXX name already exists",
"error.xxx_status_invalid": "XXX status is invalid"
```

### Step 10: gRPC/HTTP Registration (`bootstrap/services.go`)

Uncomment and fill in the gRPC and HTTP handler registration:
```go
// In registerGRPCServices():
pb.RegisterXxxServiceServer(srv, s.svc)

// In registerHTTPHandlers():
s.gateway.RegisterGatewayHandler(func(ctx context.Context, mux *runtime.ServeMux) error {
    return pb.RegisterXxxServiceHandlerServer(ctx, mux, s.svc)
})
```

## Error Handling Pattern

Always use i18n errors in service methods:
```go
func (s *XxxService) GetByID(ctx context.Context, id string) (*models.XxxModel, error) {
    model, err := s.repo.GetByID(ctx, id)
    if err != nil {
        return nil, errors.ToGRPCError(ctx, errors.BizErrCodeXxxNotFound)
    }
    return model, nil
}
```

## ID Generation Pattern

Use Snowflake short IDs for new entities:
```go
model.XxxId = NewSnowflakeID()
```

## Pagination Pattern

```go
pagination := models.BuildPagination(req.Page, req.Size, total)
```

## i18n Field Pattern

For entities with multilingual names:
```go
// Model side:
I18nNames models.LocalizedTextList `gorm:"column:i18n_names;type:json"`

// Proto → Model:
model.I18nNames = models.ConvertI18NNamesToModel(req.I18nNames)

// Model → Proto:
pbNames := models.ConvertI18NNamesToPB(model.I18nNames)
```

## Important Rules

1. **NEVER** skip any step in the checklist - all layers must be consistent
2. **ALWAYS** add locale entries for new error codes in ALL language files
3. **ALWAYS** use `errors.ToGRPCError()` for returning errors from service methods
4. **ALWAYS** use `NewSnowflakeID()` for generating entity IDs
5. **ALWAYS** match model field names with proto field names for PBMO compatibility
6. **NEVER** use raw `status.Errorf()` in service code - use the i18n error system
7. **ALWAYS** add table name constants to `models/table_names.go`
8. **ALWAYS** register new repositories in `RepositoryFactory`
9. **ALWAYS** add models to `GetAllModels()` in migrations
