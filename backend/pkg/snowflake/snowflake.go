// Package snowflake 雪花 ID 生成。
package snowflake

import (
	"fmt"
	"sync"

	sf "github.com/bwmarrin/snowflake"
)

var (
	node *sf.Node
	once sync.Once
)

// Init 初始化节点（多副本部署需保证 nodeID 不同）。
func Init(nodeID int64) error {
	var err error
	once.Do(func() {
		node, err = sf.NewNode(nodeID)
	})
	if err != nil {
		return fmt.Errorf("init snowflake: %w", err)
	}
	return nil
}

// Next 取下一个 ID。
func Next() int64 {
	if node == nil {
		_ = Init(1)
	}
	return node.Generate().Int64()
}

// NextString 取字符串形式 ID。
func NextString() string {
	if node == nil {
		_ = Init(1)
	}
	return node.Generate().String()
}
