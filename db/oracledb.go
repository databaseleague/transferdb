/*
Copyright © 2020 Marvin

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/WentaoJin/transferdb/zlog"
	"go.uber.org/zap"

	"github.com/WentaoJin/transferdb/util"

	_ "github.com/godror/godror"
)

// 创建 oracle 数据库引擎
func NewOracleDBEngine(dsn string) (*sql.DB, error) {
	sqlDB, err := sql.Open("godror", dsn)
	if err != nil {
		return sqlDB, fmt.Errorf("error on initializing oracle database connection: %v", err)
	}
	err = sqlDB.Ping()
	if err != nil {
		return sqlDB, fmt.Errorf("error on ping oracle database connection:%v", err)
	}
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(20)
	sqlDB.SetConnMaxLifetime(time.Hour)
	return sqlDB, nil
}

func (e *Engine) IsExistOracleSchema(schemaName string) error {
	schemas, err := e.getOracleSchema()
	if err != nil {
		return err
	}
	if !util.IsContainString(schemas, strings.ToUpper(schemaName)) {
		return fmt.Errorf("oracle schema [%s] isn't exist in the database", schemaName)
	}
	return nil
}

func (e *Engine) IsExistOracleTable(schemaName string, includeTables []string) error {
	tables, err := e.getOracleTable(schemaName)
	if err != nil {
		return err
	}
	ok, noExistTables := util.IsSubsetString(tables, includeTables)
	if !ok {
		return fmt.Errorf("oracle include-tables values [%v] isn't exist in the db schema [%v]", noExistTables, schemaName)
	}
	return nil
}

// 查询 Oracle 数据并按行返回对应字段以及行数据 -> 按字段类型返回行数据
func QueryOracleRows(db *sql.DB, querySQL string) ([]string, [][]string, error) {
	zlog.Logger.Info("exec sql",
		zap.String("sql", fmt.Sprintf("%v", querySQL)))
	var (
		cols       []string
		actualRows [][]string
		err        error
	)
	rows, err := db.Query(querySQL)
	if err == nil {
		defer rows.Close()
	}
	if err != nil {
		return cols, actualRows, err
	}

	cols, err = rows.Columns()
	if err != nil {
		return cols, actualRows, err
	}

	// 用于判断字段值是数字还是字符
	var columnTypes []string
	colTypes, err := rows.ColumnTypes()
	if err != nil {
		return cols, actualRows, err
	}

	for _, ct := range colTypes {
		// 数据库字段类型 DatabaseTypeName() 映射 go 类型 ScanType()
		columnTypes = append(columnTypes, ct.ScanType().String())
	}

	// Read all rows
	for rows.Next() {
		rawResult := make([][]byte, len(cols))
		result := make([]string, len(cols))
		dest := make([]interface{}, len(cols))
		for i := range rawResult {
			dest[i] = &rawResult[i]
		}

		err = rows.Scan(dest...)
		if err != nil {
			return cols, actualRows, err
		}

		for i, raw := range rawResult {
			// 注意 Oracle/Mysql NULL VS 空字符串区别
			// Oracle 空字符串与 NULL 归于一类，统一 NULL 处理 （is null 可以查询 NULL 以及空字符串值，空字符串查询无法查询到空字符串值）
			// Mysql 空字符串与 NULL 非一类，NULL 是 NULL，空字符串是空字符串（is null 只查询 NULL 值，空字符串查询只查询到空字符串值）
			// 按照 Oracle 特性来，转换同步统一转换成 NULL 即可，但需要注意业务逻辑中空字符串得写入，需要变更
			// Oracle/Mysql 对于 'NULL' 统一字符 NULL 处理，查询出来转成 NULL,所以需要判断处理
			if raw == nil {
				result[i] = "NULL"
			} else if string(raw) == "" {
				result[i] = "NULL"
			} else {
				ok := util.IsNum(string(raw))
				switch {
				case ok && columnTypes[i] != "string":
					result[i] = string(raw)
				default:
					result[i] = fmt.Sprintf("'%s'", string(raw))
				}

			}
		}
		actualRows = append(actualRows, result)
	}
	if err = rows.Err(); err != nil {
		return cols, actualRows, err
	}
	return cols, actualRows, nil
}