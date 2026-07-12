package request

type PageResult[T any] struct {
	List  []T   `json:"list"`
	Total int64 `json:"total"`
}
