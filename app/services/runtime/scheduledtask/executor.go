package scheduledtask

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode"

	"goravel/app/facades"
	"goravel/app/models"
	"goravel/app/support/apperror"
	"goravel/app/support/safehttp"
)

type ScheduledTaskHandler func(context.Context, models.JSONMap) ScheduledTaskExecutionResult

type ScheduledTaskExecutionResult struct {
	Status       string
	ExitCode     *int
	HTTPStatus   *int
	Stdout       string
	Stderr       string
	ErrorMessage string
}

var scheduledTaskHandlers = struct {
	sync.RWMutex
	items map[string]ScheduledTaskHandlerDefinition
}{items: map[string]ScheduledTaskHandlerDefinition{}}

func init() {
	MustRegisterScheduledTaskHandlerDefinition(ScheduledTaskHandlerDefinition{
		HandlerKey:       "scheduler.noop",
		Description:      "空操作健康检查处理器",
		ParameterSchema:  models.JSONMap{"type": "object", "additionalProperties": false},
		DefaultTimeout:   5,
		TenantCapability: ScheduledTaskTenantGlobalOnly,
		Handler: func(context.Context, models.JSONMap) ScheduledTaskExecutionResult {
			return ScheduledTaskExecutionResult{Status: ScheduledTaskLogStatusSuccess, Stdout: "ok"}
		},
	})
}

func RegisterScheduledTaskHandler(name string, handler ScheduledTaskHandler) {
	_ = RegisterScheduledTaskHandlerDefinition(ScheduledTaskHandlerDefinition{
		HandlerKey:       name,
		Description:      name,
		DefaultTimeout:   60,
		TenantCapability: ScheduledTaskTenantPerTenantAllowed,
		Privileged:       isPrivilegedScheduledTaskHandler(name),
		Handler:          handler,
	})
}

func UnregisterScheduledTaskHandler(name string) {
	scheduledTaskHandlers.Lock()
	defer scheduledTaskHandlers.Unlock()
	delete(scheduledTaskHandlers.items, strings.TrimSpace(name))
}

func executeScheduledTask(ctx context.Context, task models.ScheduledTask) ScheduledTaskExecutionResult {
	scope, err := NewScheduledTaskService().scheduledTaskTenantScopeFor(task)
	if err != nil {
		return taskFailure(err.Error())
	}
	return executeScheduledTaskWithScope(ctx, task, scope)
}

func executeScheduledTaskWithScope(ctx context.Context, task models.ScheduledTask, scope scheduledTaskTenantScope) ScheduledTaskExecutionResult {
	switch task.TaskType {
	case ScheduledTaskTypeURL:
		return executeURLTask(ctx, task)
	case ScheduledTaskTypeScript:
		return executeScriptTask(ctx, task, scope)
	case ScheduledTaskTypeMethod:
		return executeMethodTask(ctx, task, scope)
	case ScheduledTaskTypeBackup:
		return executeBackupTask(ctx, task, scope)
	case ScheduledTaskTypeGovernance:
		return executeGovernanceTask(ctx, task, scope)
	case ScheduledTaskTypeHandler:
		return executeRegisteredHandlerTask(ctx, task, scope)
	default:
		return taskFailure("不支持的任务类型")
	}
}

func executeURLTask(ctx context.Context, task models.ScheduledTask) ScheduledTaskExecutionResult {
	method := strings.ToUpper(jsonString(task.Payload, "method"))
	if method == "" {
		method = http.MethodGet
	}
	rawURL := jsonString(task.Payload, "url")
	parsed, err := scheduledTaskURL(rawURL)
	if err != nil {
		return taskFailure(err.Error())
	}

	var body io.Reader
	if rawBody := jsonString(task.Payload, "body"); rawBody != "" {
		body = strings.NewReader(rawBody)
	}
	req, err := http.NewRequestWithContext(ctx, method, parsed.String(), body)
	if err != nil {
		return taskFailure(err.Error())
	}
	for key, value := range jsonStringMap(task.Payload, "headers") {
		req.Header.Set(key, value)
	}

	client := scheduledTaskHTTPClient(scheduledTaskTimeout(task))
	res, err := client.Do(req)
	if err != nil {
		return taskFailure(err.Error())
	}
	defer res.Body.Close()

	content, _ := io.ReadAll(io.LimitReader(res.Body, int64(maxLogOutput(task))))
	status := ScheduledTaskLogStatusSuccess
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		status = ScheduledTaskLogStatusFailed
	}
	return ScheduledTaskExecutionResult{
		Status:     status,
		HTTPStatus: intPtr(res.StatusCode),
		Stdout:     string(content),
	}
}

func executeScriptTask(ctx context.Context, task models.ScheduledTask, scope scheduledTaskTenantScope) ScheduledTaskExecutionResult {
	command := strings.TrimSpace(jsonString(task.Payload, "command"))
	if command == "" {
		return taskFailure("脚本命令不能为空")
	}
	if !allowedScriptCommand(command) {
		return taskFailure("脚本命令必须在 storage/scripts 目录下")
	}
	command = scriptCommandPath(command)

	runCtx, cancel := context.WithTimeout(ctx, scheduledTaskTimeout(task))
	defer cancel()
	process := facades.Process().WithContext(context.Background()).Quietly().Env(scope.Env(task))
	if workdir := strings.TrimSpace(jsonString(task.Payload, "workdir")); workdir != "" {
		process = process.Path(workdir)
	}
	running, err := process.Start(command, jsonStringSlice(task.Payload, "args")...)
	if err != nil {
		return taskFailure(err.Error())
	}
	select {
	case <-running.Done():
	case <-runCtx.Done():
		_ = running.Stop(100 * time.Millisecond)
	}
	result := running.Wait()
	status := ScheduledTaskLogStatusSuccess
	errorMessage := ""
	if result.Failed() || result.Error() != nil {
		status = ScheduledTaskLogStatusFailed
		if result.Error() != nil {
			errorMessage = result.Error().Error()
		}
	}
	if runCtx.Err() != nil {
		status = ScheduledTaskLogStatusFailed
		errorMessage = runCtx.Err().Error()
	}
	return ScheduledTaskExecutionResult{
		Status:       status,
		ExitCode:     intPtr(result.ExitCode()),
		Stdout:       result.Output(),
		Stderr:       result.ErrorOutput(),
		ErrorMessage: errorMessage,
	}
}

func executeMethodTask(ctx context.Context, task models.ScheduledTask, scope scheduledTaskTenantScope) ScheduledTaskExecutionResult {
	handlerName := scheduledTaskHandlerKey(task)
	if isPrivilegedScheduledTaskHandler(handlerName) {
		return taskFailure("方法任务不允许调用特权任务处理器")
	}
	return executeHandlerDefinition(ctx, task, scope, handlerName)
}

func executeBackupTask(ctx context.Context, task models.ScheduledTask, scope scheduledTaskTenantScope) ScheduledTaskExecutionResult {
	return executeHandlerDefinition(ctx, task, scope, "scheduler.backup")
}

func executeGovernanceTask(ctx context.Context, task models.ScheduledTask, scope scheduledTaskTenantScope) ScheduledTaskExecutionResult {
	handlerName := scheduledTaskHandlerKey(task)
	if !isPrivilegedScheduledTaskHandler(handlerName) || handlerName == "scheduler.backup" {
		return taskFailure("治理任务处理器无效")
	}
	return executeHandlerDefinition(ctx, task, scope, handlerName)
}

func executeRegisteredHandlerTask(ctx context.Context, task models.ScheduledTask, scope scheduledTaskTenantScope) ScheduledTaskExecutionResult {
	return executeHandlerDefinition(ctx, task, scope, scheduledTaskHandlerKey(task))
}

func executeHandlerDefinition(ctx context.Context, task models.ScheduledTask, scope scheduledTaskTenantScope, handlerName string) ScheduledTaskExecutionResult {
	definition, ok := scheduledTaskHandlerDefinition(handlerName)
	if !ok {
		return taskFailure("治理任务处理器未注册")
	}
	payload := cloneJSONMap(task.Parameters)
	if payload == nil {
		payload = cloneJSONMap(task.Payload)
		delete(payload, "handler")
	}
	payload["task_code"] = task.Code
	payload["task_name"] = task.Name
	payload["_scheduler"] = scope.SchedulerPayload(task)
	return definition.Handler(ctx, payload)
}

func validateScheduledTaskPayload(task models.ScheduledTask) error {
	switch task.TaskType {
	case ScheduledTaskTypeURL:
		return BusinessError{Message: "不再允许创建或修改 URL 动态任务，请注册代码处理器"}
	case ScheduledTaskTypeScript:
		return BusinessError{Message: "不再允许创建或修改脚本动态任务，请注册代码处理器"}
	case ScheduledTaskTypeMethod, ScheduledTaskTypeHandler:
		if err := validateRegisteredScheduledTaskHandler(task); err != nil {
			return err
		}
		if task.TaskType == ScheduledTaskTypeMethod && isPrivilegedScheduledTaskHandler(task.HandlerKey) {
			return BusinessError{Message: "方法任务不允许调用特权任务处理器"}
		}
	case ScheduledTaskTypeBackup:
		task.HandlerKey = "scheduler.backup"
		return validateRegisteredScheduledTaskHandler(task)
	case ScheduledTaskTypeGovernance:
		if !isPrivilegedScheduledTaskHandler(task.HandlerKey) || task.HandlerKey == "scheduler.backup" {
			return BusinessError{Message: "治理任务处理器无效"}
		}
		return validateRegisteredScheduledTaskHandler(task)
	default:
		return BusinessError{Message: "任务类型不支持"}
	}
	return nil
}

func validateRegisteredScheduledTaskHandler(task models.ScheduledTask) error {
	handlerKey := scheduledTaskHandlerKey(task)
	if handlerKey == "" {
		return BusinessError{Message: "任务处理器不能为空"}
	}
	definition, ok := scheduledTaskHandlerDefinition(handlerKey)
	if !ok {
		return BusinessError{Message: "任务处理器未注册"}
	}
	if task.Scope == ScheduledTaskScopePerTenant && definition.TenantCapability == ScheduledTaskTenantGlobalOnly {
		return BusinessError{Message: "任务处理器仅支持全局作用域"}
	}
	parameters := task.Parameters
	if parameters == nil {
		parameters = cloneJSONMap(task.Payload)
		delete(parameters, "handler")
	}
	if err := definition.ValidateParameters(parameters); err != nil {
		return BusinessError{Message: err.Error()}
	}
	return nil
}

func scheduledTaskHandlerKey(task models.ScheduledTask) string {
	if handlerKey := strings.TrimSpace(task.HandlerKey); handlerKey != "" {
		return handlerKey
	}
	if task.TaskType == ScheduledTaskTypeBackup {
		return "scheduler.backup"
	}
	return strings.TrimSpace(jsonString(task.Payload, "handler"))
}

func ValidateScheduledTaskPayload(task models.ScheduledTask) error {
	return validateScheduledTaskPayload(task)
}

func ScheduledTaskUsesPrivilegedHandler(taskType string, payload models.JSONMap) bool {
	return strings.TrimSpace(taskType) == ScheduledTaskTypeMethod &&
		isPrivilegedScheduledTaskHandler(jsonString(payload, "handler"))
}

func isPrivilegedScheduledTaskHandler(name string) bool {
	switch strings.TrimSpace(name) {
	case "scheduler.backup":
		return true
	case "scheduler.tenant_retention", "scheduler.tenant_isolation_verify":
		return true
	default:
		return false
	}
}

func IsPrivilegedScheduledTaskHandler(name string) bool {
	return isPrivilegedScheduledTaskHandler(name)
}

func allowedScriptCommand(command string) bool {
	return scriptCommandPath(command) != ""
}

func scheduledTaskHandlerRegistered(name string) bool {
	_, ok := scheduledTaskHandler(name)
	return ok
}

func scheduledTaskHandler(name string) (ScheduledTaskHandler, bool) {
	definition, ok := scheduledTaskHandlerDefinition(name)
	return definition.Handler, ok
}

func cloneJSONMap(value models.JSONMap) models.JSONMap {
	result := models.JSONMap{}
	for key, item := range value {
		result[key] = item
	}
	return result
}

func scriptCommandPath(command string) string {
	command = strings.TrimSpace(command)
	if hasShellSyntax(command) {
		return ""
	}
	cleaned := filepath.Clean(command)
	if cleaned == "." {
		return ""
	}
	root := filepath.Clean(facades.App().BasePath("storage/scripts"))
	if filepath.IsAbs(cleaned) {
		return realScriptPath(root, cleaned)
	}
	path := filepath.Clean(facades.App().BasePath(cleaned))
	return realScriptPath(root, path)
}

func realScriptPath(root string, path string) string {
	if !isPathInside(root, path) {
		return ""
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		realRoot = root
	}
	realPath, err := filepath.EvalSymlinks(path)
	if err == nil {
		if !isPathInside(realRoot, realPath) {
			return ""
		}
		return realPath
	}
	if !os.IsNotExist(err) {
		return ""
	}
	realParent, err := realExistingAncestor(path)
	if err != nil || !isPathInsideOrEqual(realRoot, realParent) {
		return ""
	}
	return path
}

func realExistingAncestor(path string) (string, error) {
	for {
		if _, err := os.Lstat(path); err == nil {
			return filepath.EvalSymlinks(path)
		} else if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(path)
		if parent == path {
			return "", os.ErrNotExist
		}
		path = parent
	}
}

func hasShellSyntax(command string) bool {
	for _, value := range command {
		if unicode.IsSpace(value) {
			return true
		}
		switch value {
		case '&', '|', ';', '<', '>', '$', '`', '\\', '"', '\'':
			return true
		}
	}
	if runtime.GOOS == "windows" && strings.Contains(command, "%") {
		return true
	}
	return false
}

func isPathInside(root string, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." || rel == ".." {
		return false
	}
	return !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func isPathInsideOrEqual(root string, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." {
		return false
	}
	return rel == "." || !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func taskFailure(message string) ScheduledTaskExecutionResult {
	return ScheduledTaskExecutionResult{Status: ScheduledTaskLogStatusFailed, ErrorMessage: message}
}

func Failure(message string) ScheduledTaskExecutionResult {
	return taskFailure(message)
}

func scheduledTaskTimeout(task models.ScheduledTask) time.Duration {
	seconds := task.TimeoutSeconds
	if seconds < 1 {
		seconds = 60
	}
	return time.Duration(seconds) * time.Second
}

func maxLogOutput(task models.ScheduledTask) int {
	if task.MaxLogOutput < 1 {
		return 4000
	}
	return task.MaxLogOutput
}

func trimExecutionResult(result ScheduledTaskExecutionResult, limit int) ScheduledTaskExecutionResult {
	result.Stdout = trimString(result.Stdout, limit)
	result.Stderr = trimString(result.Stderr, limit)
	result.ErrorMessage = trimString(result.ErrorMessage, limit)
	return result
}

func trimString(value string, limit int) string {
	if limit < 1 || len(value) <= limit {
		return value
	}
	return value[:limit]
}

func jsonStringMap(data models.JSONMap, key string) map[string]string {
	result := map[string]string{}
	raw, ok := data[key].(map[string]any)
	if !ok {
		return result
	}
	for k, v := range raw {
		result[k] = fmt.Sprint(v)
	}
	return result
}

func jsonStringSlice(data models.JSONMap, key string) []string {
	raw, ok := data[key].([]any)
	if !ok {
		return nil
	}
	values := make([]string, 0, len(raw))
	for _, value := range raw {
		values = append(values, fmt.Sprint(value))
	}
	return values
}

func intPtr(value int) *int {
	return &value
}

func scheduledTaskURL(raw string) (*url.URL, error) {
	return safehttp.URL(raw, scheduledTaskOutboundHTTPPolicy())
}

func scheduledTaskHTTPClient(timeout time.Duration) http.Client {
	return safehttp.Client(timeout, scheduledTaskOutboundHTTPPolicy())
}

func scheduledTaskOutboundHTTPPolicy() safehttp.Policy {
	return safehttp.Policy{
		InvalidURL: func() error {
			return apperror.BusinessError{Message: "URL 任务地址必须为 http 或 https"}
		},
		UnresolvedHost: func() error {
			return apperror.BusinessError{Message: "URL 任务地址无法解析"}
		},
		InvalidAddress: func() error {
			return apperror.BusinessError{Message: "URL 任务地址无效"}
		},
		ValidateURL: func(parsed *url.URL) error {
			if parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
				return apperror.BusinessError{Message: "URL 任务地址必须为 http 或 https"}
			}
			if parsed.User != nil {
				return apperror.BusinessError{Message: "URL 任务地址不允许包含认证信息"}
			}
			return nil
		},
		ValidateTarget: func(_ *url.URL, ips []net.IP) error {
			if len(ips) == 0 {
				return apperror.BusinessError{Message: "URL 任务地址无法解析"}
			}
			for _, ip := range ips {
				if safehttp.IsPrivateIP(ip) {
					return apperror.BusinessError{Message: "URL 任务地址不允许指向内网或本机地址"}
				}
			}
			return nil
		},
	}
}
