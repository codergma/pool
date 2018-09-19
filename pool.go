package pool

import "errors"

var (
	ErrClose = errors.New("pool is closed")
)

//Pool 连接池接口
type Pool interface {
	Get() (interface{}, error)
	Put(interface{}) error
	Close(interface{}) error
	Release()
	Len() int
}
