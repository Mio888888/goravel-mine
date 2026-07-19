package services

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
	items map[string]ScheduledTaskHandler
}{items: map[string]ScheduledTaskHandler{}}

func init() {
	RegisterScheduledTaskHandler("scheduler.noop", func(context.Context, models.JSONMap) ScheduledTaskExecutionResult {
		return ScheduledTaskExecutionResult{Status: ScheduledTaskLogStatusSuccess, Stdout: "ok"}
	})
	RegisterScheduledTaskHandler("scheduler.tenant_retention", tenantRetentionScheduledTaskHandler)
	RegisterScheduledTaskHandler("scheduler.tenant_isolation_verify", tenantIsolationScheduledTaskHandler)
}

func RegisterScheduledTaskHandler(name string, handler ScheduledTaskHandler) {
	name = strings.TrimSpace(name)
	if name == "" || handler == nil {
		return
	}
	scheduledTaskHandlers.Lock()
	defer scheduledTaskHandlers.Unlock()
	scheduledTaskHandlers.items[name] = handler
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
	handlerName := strings.TrimSpace(jsonString(task.Payload, "handler"))
	if isPrivilegedScheduledTaskHandler(handlerName) {
		return taskFailure("方法任务不允许调用特权任务处理器")
	}
	handler, ok := scheduledTaskHandler(handlerName)
	if !ok {
		return taskFailure("任务方法未注册")
	}
	payload := cloneJSONMap(task.Payload)
	payload["_scheduler"] = scope.SchedulerPayload(task)
	return handler(ctx, payload)
}

func executeBackupTask(ctx context.Context, task models.ScheduledTask, scope scheduledTaskTenantScope) ScheduledTaskExecutionResult {
	handler, ok := scheduledTaskHandler("scheduler.backup")
	if !ok {
		return taskFailure("备份任务未配置处理器")
	}
	payload := cloneJSONMap(task.Payload)
	payload["task_code"] = task.Code
	payload["task_name"] = task.Name
	payload["_scheduler"] = scope.SchedulerPayload(task)
	return handler(ctx, payload)
}

func executeGovernanceTask(ctx context.Context, task models.ScheduledTask, scope scheduledTaskTenantScope) ScheduledTaskExecutionResult {
	handlerName := strings.TrimSpace(jsonString(task.Payload, "handler"))
	if !isPrivilegedScheduledTaskHandler(handlerName) || handlerName == "scheduler.backup" {
		return taskFailure("治理任务处理器无效")
	}
	handler, ok := scheduledTaskHandler(handlerName)
	if !ok {
		return taskFailure("治理任务处理器未注册")
	}
	payload := cloneJSONMap(task.Payload)
	payload["_scheduler"] = scope.SchedulerPayload(task)
	return handler(ctx, payload)
}

func validateScheduledTaskPayload(task models.ScheduledTask) error {
	switch task.TaskType {
	case ScheduledTaskTypeURL:
		rawURL := jsonString(task.Payload, "url")
		if _, err := scheduledTaskURL(rawURL); err != nil {
			return BusinessError{Message: "URL 任务地址必须为 http 或 https"}
		}
	case ScheduledTaskTypeScript:
		command := jsonString(task.Payload, "command")
		if !allowedScriptCommand(command) {
			return BusinessError{Message: "脚本命令必须在 storage/scripts 目录下"}
		}
	case ScheduledTaskTypeMethod:
		handlerName := strings.TrimSpace(jsonString(task.Payload, "handler"))
		if handlerName == "" {
			return BusinessError{Message: "方法任务必须配置 handler"}
		}
		if isPrivilegedScheduledTaskHandler(handlerName) {
			return BusinessError{Message: "方法任务不允许调用特权任务处理器"}
		}
	case ScheduledTaskTypeBackup:
		if !scheduledTaskHandlerRegistered("scheduler.backup") {
			return BusinessError{Message: "备份任务必须先注册真实处理器"}
		}
	case ScheduledTaskTypeGovernance:
		return BusinessError{Message: "治理任务仅允许系统预置"}
	default:
		return BusinessError{Message: "任务类型不支持"}
	}
	return nil
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

func allowedScriptCommand(command string) bool {
	return scriptCommandPath(command) != ""
}

func scheduledTaskHandlerRegistered(name string) bool {
	_, ok := scheduledTaskHandler(name)
	return ok
}

func scheduledTaskHandler(name string) (ScheduledTaskHandler, bool) {
	scheduledTaskHandlers.RLock()
	defer scheduledTaskHandlers.RUnlock()
	handler, ok := scheduledTaskHandlers.items[strings.TrimSpace(name)]
	return handler, ok
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
	return safeOutboundURL(raw, scheduledTaskOutboundHTTPPolicy())
}

func scheduledTaskHTTPClient(timeout time.Duration) http.Client {
	return safeOutboundHTTPClient(timeout, scheduledTaskOutboundHTTPPolicy())
}

func scheduledTaskOutboundHTTPPolicy() outboundHTTPPolicy {
	return outboundHTTPPolicy{
		invalidURL: func() error {
			return BusinessError{Message: "URL 任务地址必须为 http 或 https"}
		},
		unresolvedHost: func() error {
			return BusinessError{Message: "URL 任务地址无法解析"}
		},
		invalidAddress: func() error {
			return BusinessError{Message: "URL 任务地址无效"}
		},
		validateURL: func(parsed *url.URL) error {
			if parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
				return BusinessError{Message: "URL 任务地址必须为 http 或 https"}
			}
			if parsed.User != nil {
				return BusinessError{Message: "URL 任务地址不允许包含认证信息"}
			}
			return nil
		},
		validateTarget: func(_ *url.URL, ips []net.IP) error {
			if len(ips) == 0 {
				return BusinessError{Message: "URL 任务地址无法解析"}
			}
			for _, ip := range ips {
				if isPrivateOutboundIP(ip) {
					return BusinessError{Message: "URL 任务地址不允许指向内网或本机地址"}
				}
			}
			return nil
		},
	}
}
