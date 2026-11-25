package db

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type transferTable interface {
	name() string
	export(*gorm.DB, *transferOutput, *transferState) error
	restore(*gorm.DB, *transferInput, *transferState, bool) error
}

type transferModel interface{ TableName() string }

// A fixed, typed registry defines the wire format. No table name, column name
// or SQL expression from a transfer is sent to the database.
type transferRows[T transferModel] struct {
	order        string
	numericOrder bool
	maxRows      int // zero means no table-specific retention limit
	singleton    bool
	validate     func(*T, *transferState) bool
	insert       func(*gorm.DB, *T) error
	blob         func(*T) *[]byte
	blobLimit    int
	selectRow    func(*gorm.DB) *gorm.DB
}

func (section transferRows[T]) name() string {
	var model T
	return model.TableName()
}

func (section transferRows[T]) export(tx *gorm.DB, out *transferOutput, state *transferState) error {
	if err := out.frame([]byte(section.name()), transferManifestLimit); err != nil {
		return err
	}
	query := tx.Model(new(T))
	column := clause.Column{Name: section.order}
	if section.numericOrder {
		query = query.Order(clause.OrderByColumn{Column: column})
	} else {
		// Text keys use byte order on both drivers, independent of DB locale.
		collation := `"BINARY"`
		if tx.Dialector.Name() == Postgres {
			collation = `"C"`
		}
		query = query.Order(clause.OrderBy{Expression: clause.Expr{SQL: "? COLLATE " + collation, Vars: []any{column}}})
	}
	if section.selectRow != nil {
		query = section.selectRow(query)
	}
	rows, err := query.Rows()
	if err != nil {
		return errTransferDatabase
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		count++
		if section.maxRows != 0 && count > section.maxRows {
			return errTransferFormat
		}
		var row T
		if err := tx.ScanRows(rows, &row); err != nil {
			return errTransferDatabase
		}
		if !section.validate(&row, state) {
			return errTransferFormat
		}
		var data []byte
		if section.blob != nil {
			field := section.blob(&row)
			data, *field = *field, nil
		}
		if err := out.json(&row, transferRecordLimit); err != nil {
			return err
		}
		if section.blob != nil {
			if err := out.frame(data, section.blobLimit); err != nil {
				return err
			}
		}
	}
	if rows.Err() != nil {
		return errTransferDatabase
	}
	if section.singleton && count != 1 {
		return errTransferFormat
	}
	return out.frame(nil, transferRecordLimit)
}

func (section transferRows[T]) restore(tx *gorm.DB, in *transferInput, state *transferState, retry bool) error {
	table, err := in.frame(transferManifestLimit)
	if err != nil {
		return err
	}
	if string(table) != section.name() {
		return errTransferFormat
	}
	for count := 0; ; count++ {
		data, err := in.frame(transferRecordLimit)
		if err != nil {
			return err
		}
		if data == nil {
			if section.singleton && count != 1 {
				return errTransferFormat
			}
			return nil
		}
		if section.maxRows != 0 && count >= section.maxRows {
			return errTransferFormat
		}
		if err := section.restoreRow(tx, in, data, state, retry); err != nil {
			return err
		}
	}
}

func (section transferRows[T]) restoreRow(tx *gorm.DB, in *transferInput, data []byte, state *transferState, retry bool) error {
	var row T
	if !decodeTransferJSON(data, &row) {
		return errTransferFormat
	}
	if section.blob != nil {
		field := section.blob(&row)
		if *field != nil {
			return errTransferFormat
		}
		var err error
		*field, err = in.frame(section.blobLimit)
		if err != nil {
			return err
		}
	}
	if !section.validate(&row, state) {
		return errTransferFormat
	}
	if retry {
		return nil // Import verifies the source footer and reads back the destination.
	}
	var err error
	if section.insert != nil {
		err = section.insert(tx, &row)
	} else {
		err = tx.Create(&row).Error
	}
	if err != nil {
		return errTransferDatabase
	}
	return nil
}
