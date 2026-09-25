// Package gorm is a minimal stand-in for gorm.io/gorm: only the method
// shapes the gormerr analyzer keys on.
package gorm

type DB struct {
	Error        error
	RowsAffected int64
}

func (db *DB) Where(query interface{}, args ...interface{}) *DB { return db }
func (db *DB) Model(value interface{}) *DB                      { return db }
func (db *DB) Raw(sql string, values ...interface{}) *DB        { return db }
func (db *DB) Unscoped() *DB                                    { return db }
func (db *DB) Begin() *DB                                       { return db }
func (db *DB) Rollback() *DB                                    { return db }
func (db *DB) Commit() *DB                                      { return db }

func (db *DB) Create(value interface{}) *DB                             { return db }
func (db *DB) Save(value interface{}) *DB                               { return db }
func (db *DB) First(dest interface{}, conds ...interface{}) *DB         { return db }
func (db *DB) Find(dest interface{}, conds ...interface{}) *DB          { return db }
func (db *DB) Update(column string, value interface{}) *DB              { return db }
func (db *DB) Updates(values interface{}) *DB                           { return db }
func (db *DB) UpdateColumn(column string, value interface{}) *DB        { return db }
func (db *DB) Delete(value interface{}, conds ...interface{}) *DB       { return db }
func (db *DB) Exec(sql string, values ...interface{}) *DB               { return db }
func (db *DB) Count(count *int64) *DB                                   { return db }
func (db *DB) Scan(dest interface{}) *DB                                { return db }
func (db *DB) Pluck(column string, dest interface{}) *DB                { return db }
func (db *DB) FirstOrCreate(dest interface{}, conds ...interface{}) *DB { return db }
func (db *DB) Transaction(fc func(tx *DB) error) error                  { return nil }

// Take does not actually exist on the real *gorm.DB with this signature;
// it stands in here only so the analyzer's test suite can exercise a
// finisher-named method whose *receiver* is genuinely *gorm.DB but whose
// *result* is a pointer to an unnamed type (isGormDBPtr's Named assertion
// failing on the result, as opposed to the receiver).
func (db *DB) Take() *[]int { return nil }
