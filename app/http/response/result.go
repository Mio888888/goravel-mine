package response

type Result struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

func Success(data any) Result {
	return Result{
		Code:    CodeSuccess,
		Message: "成功",
		Data:    emptyArrayIfNil(data),
	}
}

func SuccessEmpty() Result {
	return Success([]any{})
}

func Error(code int, message string, data any) Result {
	return Result{
		Code:    code,
		Message: message,
		Data:    emptyArrayIfNil(data),
	}
}

func emptyArrayIfNil(data any) any {
	if data == nil {
		return []any{}
	}

	return data
}
