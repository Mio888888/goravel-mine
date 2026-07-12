package response

import (
	"encoding/json"
	"time"
)

const mineTimeLayout = "2006-01-02 15:04:05"

type MineTime time.Time

func (t MineTime) MarshalJSON() ([]byte, error) {
	value := time.Time(t).Format(mineTimeLayout)

	return json.Marshal(value)
}
