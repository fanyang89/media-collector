package bilibili

import (
	"github.com/cockroachdb/errors"
	"github.com/xuri/excelize/v2"
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

func (h *History) ExportExcel(filePath string) error {
	f, err := excelize.OpenFile(filePath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	const sheetName = "History"
	sheetIdx, err := f.NewSheet(sheetName)
	if err != nil {
		return err
	}
	f.SetActiveSheet(sheetIdx)

	err = f.DeleteSheet("Sheet1")
	if err != nil {
		return err
	}

	idx := 1
	cell, err := excelize.CoordinatesToCellName(1, idx)
	if err != nil {
		return err
	}
	idx++

	err = f.SetSheetRow(sheetName, cell, []interface{}{
		"BVID", "Author", "Title", "Keyword", "Tags", "FileName",
	})
	if err != nil {
		return err
	}

	rows, err := h.db.Model(&HistoryEntry{}).Rows()
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var entry HistoryEntry
		err = h.db.ScanRows(rows, &entry)
		if err != nil {
			return err
		}

		cell, err = excelize.CoordinatesToCellName(1, idx)
		if err != nil {
			return err
		}
		idx++

		err = f.SetSheetRow(sheetName, cell, []interface{}{
			entry.Bvid, entry.Author, entry.Title, entry.Keyword, entry.Tags, entry.FileName,
		})
		if err != nil {
			return err
		}
	}

	return rows.Err()
}
