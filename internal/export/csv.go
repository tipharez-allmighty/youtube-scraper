// Package export.
package export

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"reflect"
	"time"
)

type SelectorFunc[T any] func(jobID string, limit, offset int) ([]T, int, error)

func WriteCSV[T any](ctx context.Context, filename string, jobID string, limit int, sfunc SelectorFunc[T]) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	writer := csv.NewWriter(file)
	defer writer.Flush()

	offset := 0
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		data, nextOffset, err := sfunc(jobID, limit, offset)
		if err != nil {
			return err
		}
		if len(data) == 0 {
			break
		}
		var rowsToWrite [][]string
		if offset == 0 {
			headers, err := getHeader(data[0])
			if err != nil {
				return fmt.Errorf("failed to fetch csv header from the struct: %w", err)
			}
			rowsToWrite = append(rowsToWrite, headers)
		}
		for _, val := range data {
			row, err := getRow(val)
			if err != nil {
				return fmt.Errorf("failed to fetch data from the struct: %w", err)
			}
			rowsToWrite = append(rowsToWrite, row)
		}
		if err := writer.WriteAll(rowsToWrite); err != nil {
			return err
		}
		if len(data) < limit {
			break
		}
		offset = nextOffset
	}
	return nil
}

func getHeader(i any) ([]string, error) {
	v := reflect.ValueOf(i)
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil, fmt.Errorf("expected struct, got %s", v.Kind())
	}
	vf := reflect.VisibleFields(v.Type())
	var headers []string
	for _, field := range vf {
		if field.Anonymous {
			continue
		}
		headers = append(headers, field.Name)
	}
	return headers, nil
}

func getRow(i any) ([]string, error) {
	v := reflect.ValueOf(i)
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil, fmt.Errorf("expected struct, got %s", v.Kind())
	}
	var row []string
	vf := reflect.VisibleFields(v.Type())
	for _, field := range vf {
		if field.Anonymous {
			continue
		}
		row = append(row, formatField(v.FieldByIndex(field.Index)))
	}
	return row, nil
}

func formatField(fv reflect.Value) string {
	if fv.Kind() == reflect.Pointer {
		if fv.IsNil() {
			return ""
		}
		fv = fv.Elem()
	}
	if t, ok := fv.Interface().(time.Time); ok {
		return t.Format(time.RFC3339)
	}
	return fmt.Sprintf("%v", fv.Interface())
}

