package services

import (
	"context"
	"strings"
	"time"

	contractsorm "github.com/goravel/framework/contracts/database/orm"
	"goravel/app/http/request"
	"goravel/app/models"
)

const logTimeLayout = "2006-01-02 15:04:05"

type LogAdminService struct {
	ctx        context.Context
	connection string
}

type LoginLogRow struct {
	ID        uint64 `json:"id"`
	Username  string `json:"username"`
	IP        string `json:"ip"`
	OS        string `json:"os"`
	Browser   string `json:"browser"`
	Status    int16  `json:"status"`
	Message   string `json:"message"`
	LoginTime string `json:"login_time"`
	Remark    string `json:"remark"`
}

type OperationLogRow struct {
	ID          uint64 `json:"id"`
	Username    string `json:"username"`
	Method      string `json:"method"`
	Router      string `json:"router"`
	ServiceName string `json:"service_name"`
	IP          string `json:"ip"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
	Remark      string `json:"remark"`
}

type OperationLogPayload struct {
	Username    string
	Method      string
	Router      string
	ServiceName string
	IP          string
	Connection  string
}

func NewLogAdminService() *LogAdminService {
	return &LogAdminService{}
}

func NewLogAdminServiceForTenant(tenant Tenant) *LogAdminService {
	return &LogAdminService{connection: TenantConnectionName(tenant)}
}

func NewLogAdminServiceForConnection(connection string) *LogAdminService {
	return &LogAdminService{connection: connection}
}

func (s *LogAdminService) WithContext(ctx context.Context) *LogAdminService {
	clone := *s
	clone.ctx = contextOrBackground(ctx)
	return &clone
}

func (s *LogAdminService) orm() contractsorm.Orm {
	return OrmForConnectionWithContext(s.ctx, s.connection)
}

func (s *LogAdminService) ListLoginLogs(filters map[string]string, page, pageSize int) (request.PageResult[LoginLogRow], error) {
	query := loginLogFilters(s.orm().Query().Table("user_login_log"), filters)
	total, err := query.Count()
	if err != nil {
		return request.PageResult[LoginLogRow]{}, err
	}

	logs := make([]models.UserLoginLog, 0)
	err = query.OrderByDesc("id").Offset((page - 1) * pageSize).Limit(pageSize).Get(&logs)
	if err != nil {
		return request.PageResult[LoginLogRow]{}, err
	}

	return request.PageResult[LoginLogRow]{List: loginLogRows(logs), Total: total}, nil
}

func (s *LogAdminService) ListOperationLogs(filters map[string]string, page, pageSize int) (request.PageResult[OperationLogRow], error) {
	query := operationLogFilters(s.orm().Query().Table("user_operation_log"), filters)
	total, err := query.Count()
	if err != nil {
		return request.PageResult[OperationLogRow]{}, err
	}

	logs := make([]models.UserOperationLog, 0)
	err = query.OrderByDesc("id").Offset((page - 1) * pageSize).Limit(pageSize).Get(&logs)
	if err != nil {
		return request.PageResult[OperationLogRow]{}, err
	}

	return request.PageResult[OperationLogRow]{List: operationLogRows(logs), Total: total}, nil
}

func (s *LogAdminService) WriteOperationLog(payload OperationLogPayload) error {
	now := time.Now()
	return s.orm().Query().Create(&models.UserOperationLog{
		Username:    payload.Username,
		Method:      strings.ToUpper(payload.Method),
		Router:      payload.Router,
		ServiceName: payload.ServiceName,
		IP:          payload.IP,
		Timestamps:  models.Timestamps{CreatedAt: now, UpdatedAt: now},
	})
}

func loginLogFilters(query contractsorm.Query, filters map[string]string) contractsorm.Query {
	query = equalFilter(query, "username", filters["username"])
	query = equalFilter(query, "ip", filters["ip"])
	query = equalFilter(query, "os", filters["os"])
	query = equalFilter(query, "browser", filters["browser"])
	query = equalFilter(query, "status", filters["status"])
	query = equalFilter(query, "message", filters["message"])
	return equalFilter(query, "remark", filters["remark"])
}

func operationLogFilters(query contractsorm.Query, filters map[string]string) contractsorm.Query {
	query = equalFilter(query, "username", filters["username"])
	query = equalFilter(query, "method", strings.ToUpper(filters["method"]))
	query = equalFilter(query, "router", filters["router"])
	query = equalFilter(query, "service_name", filters["service_name"])
	return equalFilter(query, "ip", filters["ip"])
}

func equalFilter(query contractsorm.Query, column, value string) contractsorm.Query {
	if strings.TrimSpace(value) == "" {
		return query
	}
	return query.Where(column, value)
}

func loginLogRows(logs []models.UserLoginLog) []LoginLogRow {
	rows := make([]LoginLogRow, 0, len(logs))
	for _, log := range logs {
		rows = append(rows, LoginLogRow{
			ID: log.ID, Username: log.Username, IP: log.IP, OS: log.OS,
			Browser: log.Browser, Status: log.Status, Message: log.Message,
			LoginTime: formatLogTime(log.LoginTime), Remark: log.Remark,
		})
	}
	return rows
}

func operationLogRows(logs []models.UserOperationLog) []OperationLogRow {
	rows := make([]OperationLogRow, 0, len(logs))
	for _, log := range logs {
		rows = append(rows, OperationLogRow{
			ID: log.ID, Username: log.Username, Method: log.Method, Router: log.Router,
			ServiceName: log.ServiceName, IP: log.IP, CreatedAt: formatLogTime(log.CreatedAt),
			UpdatedAt: formatLogTime(log.UpdatedAt), Remark: log.Remark,
		})
	}
	return rows
}

func formatLogTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(logTimeLayout)
}
