package bilibili

import (
	"github.com/cockroachdb/errors"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"moul.io/zapgorm2"
)

type History struct {
	db *gorm.DB
}

type HistoryEntry struct {
	Bvid     string `json:"bvid"`
	Author   string `json:"author"`
	Title    string `json:"title"`
	Keyword  string `json:"keyword"`
	Tags     string `json:"tags"`
	FileName string `json:"file_name"`
}

func NewHistory(dsn string) (*History, error) {
	log := zapgorm2.New(zap.L())
	log.IgnoreRecordNotFoundError = true
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: log,
	})
	if err != nil {
		return nil, err
	}

	err = db.AutoMigrate(&HistoryEntry{})
	if err != nil {
		return nil, err
	}

	return &History{db: db}, nil
}

func (h *History) Save(entry *HistoryEntry) error {
	return h.db.Clauses(clause.OnConflict{UpdateAll: true}).Create(entry).Error
}

func (h *History) IsDownloaded(bvid string) (ok bool, err error) {
	var entry HistoryEntry
	err = h.db.First(&entry, "bvid = ?", bvid).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = nil
		}
	} else {
		ok = true
	}
	return
}
