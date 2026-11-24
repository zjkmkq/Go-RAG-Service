package repository

import (
	"database/sql"
	"fmt"

	"go-rag-service/internal/config"

	_ "github.com/lib/pq"
)

var DB *sql.DB

func InitDB(cfg *config.Config) (*sql.DB, error) {
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		cfg.Database.Host,
		cfg.Database.Port,
		cfg.Database.User,
		cfg.Database.Password,
		cfg.Database.DBName,
	)

	var err error
	DB, err = sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("数据库参数错误: %w", err)
	}

	if err = DB.Ping(); err != nil {
		return nil, fmt.Errorf("数据库连接失败: %w", err)
	}

	DB.SetMaxOpenConns(25)
	DB.SetMaxIdleConns(5)

	return DB, nil
}

func CloseDB() {
	if DB != nil {
		DB.Close()
	}
}
