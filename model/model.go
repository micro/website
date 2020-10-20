// Package model implements convenience methods for
// managing indexes on top of the Store.
// See this doc for the general idea https://github.com/m3o/dev/blob/feature/storeindex/design/auto-indexes.md
// Prior art/Inspirations from github.com/gocassa/gocassa, which
// is a similar package on top an other KV store (Cassandra/gocql)
package model

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/micro/micro/v3/service/store"
)

const (
	queryTypeEq = "eq"
	indexTypeEq = "eq"
)

type db struct {
	store   store.Store
	indexes []Index
	entity  interface{}
	fields  []string
	debug   bool
}

func (d *db) Save(instance interface{}) error {
	// @todo replace this hack with reflection
	js, err := json.Marshal(instance)
	if err != nil {
		return err
	}
	m := map[string]interface{}{}
	err = json.Unmarshal(js, &m)
	if err != nil {
		return err
	}
	id, ok := m["ID"].(string)
	if !ok || len(id) == 0 {
		id, ok = m["id"].(string)
		if !ok || len(id) == 0 {
			return fmt.Errorf("ID of objects must marshal to JSON key 'ID' or 'id'")
		}
	}
	if d.debug {
		fmt.Printf("Saving key %v, value: %v\n", id, string(js))
	}
	err = d.store.Write(&store.Record{
		Key:   id,
		Value: js,
	})
	if err != nil {
		return err
	}
	for _, index := range d.indexes {
		k := indexToSaveKey(index, id, m)
		if d.debug {
			fmt.Printf("Saving key %v, value: %v\n", k, string(js))
		}
		err = d.store.Write(&store.Record{
			Key:   k,
			Value: js,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *db) List(query Query, resultSlicePointer interface{}) error {
	if len(d.indexes) == 0 {
		return errors.New("No indexes found")
	}
	for _, index := range d.indexes {
		if indexMatchesQuery(index, query) {
			k := queryToListKey(query)
			if d.debug {
				fmt.Printf("Listing key %v\n", k)
			}
			recs, err := d.store.Read(k, store.ReadPrefix())
			if err != nil {
				return err
			}
			// @todo speed this up with an actual buffer
			jsBuffer := []byte("[")
			for i, rec := range recs {
				jsBuffer = append(jsBuffer, rec.Value...)
				if i < len(recs)-1 {
					jsBuffer = append(jsBuffer, []byte(",")...)
				}
			}
			jsBuffer = append(jsBuffer, []byte("]")...)
			return json.Unmarshal(jsBuffer, resultSlicePointer)
		}
	}
	return fmt.Errorf("Query type %v, field %v does not match any indexes", query.Type, query.FieldName)
}

func indexMatchesQuery(i Index, q Query) bool {
	if i.Type == q.Type && i.Ordering == q.Ordering {
		return true
	}
	return false
}

func queryToListKey(q Query) string {
	return fmt.Sprintf("by%v:%v", q.FieldName, q.Value)
}

func indexToSaveKey(i Index, id string, m map[string]interface{}) string {
	return fmt.Sprintf("by%v:%v:%v", i.FieldName, m[i.FieldName], id)
}

// DB represents a place where data can be saved to and
// queried from.
type DB interface {
	Save(interface{}) error
	List(query Query, resultPointer interface{}) error
}

func NewDB(store store.Store, entity interface{}, indexes []Index) DB {
	return &db{
		store, indexes, entity, nil, true,
	}
}

type Index struct {
	FieldName string
	Type      string // eg. equality
	Ordering  bool   // ASC or DESC ordering
}

func Indexes(indexes ...Index) []Index {
	return indexes
}

// ByEq constructs an equiality index on `fieldName`
func ByEq(fieldName string) Index {
	return Index{
		FieldName: fieldName,
		Type:      indexTypeEq,
	}
}

type Query struct {
	Type      string
	FieldName string
	Value     interface{}
	// false = ascending order, true = descending order
	Ordering bool
}

func (q Query) Ord(desc bool) Query {
	return Query{
		Type:      q.Type,
		FieldName: q.FieldName,
		Value:     q.Value,
		Ordering:  desc,
	}
}

// Eq is an equality query by `fieldName`
func Eq(fieldName string, value interface{}) Query {
	return Query{
		Type:      queryTypeEq,
		FieldName: fieldName,
		Value:     value,
	}
}
