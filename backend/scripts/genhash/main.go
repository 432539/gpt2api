// Command genhash 生成 bcrypt 哈希。
//
// 用法： go run ./scripts/genhash <password>
package main

import (
	"fmt"
	"os"

	"golang.org/x/crypto/bcrypt"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: genhash <password>")
		os.Exit(1)
	}
	h, err := bcrypt.GenerateFromPassword([]byte(os.Args[1]), 12)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(h))
}
