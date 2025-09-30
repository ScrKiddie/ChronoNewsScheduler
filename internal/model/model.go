package model

type File struct {
	ID             int32   `gorm:"column:id;primaryKey;type:integer;autoIncrement;not null"`
	CreatedAt      int64   `gorm:"column:created_at;autoCreateTime:unixtime"`
	UpdatedAt      int64   `gorm:"column:updated_at;autoCreateTime:unixtime;autoUpdateTime:unixtime"`
	Name           string  `gorm:"column:name;type:varchar(255);index"`
	Type           string  `gorm:"column:type;type:file_type;default:'attachment';index"`
	Status         string  `gorm:"column:status;type:file_status;default:'pending';index"`
	FailedAttempts int     `gorm:"column:failed_attempts;default:0"`
	LastError      *string `gorm:"column:last_error;type:varchar(255)"`
	UsedByPostID   *int32  `gorm:"column:used_by_post_id;index"`
}

func (File) TableName() string {
	return "file"
}

type DeadLetterQueue struct {
	ID           int32  `gorm:"column:id;primaryKey;type:integer;autoIncrement;not null"`
	CreatedAt    int64  `gorm:"column:created_at;autoCreateTime:unixtime"`
	UpdatedAt    int64  `gorm:"column:updated_at;autoCreateTime:unixtime;autoUpdateTime:unixtime"`
	FileID       int32  `gorm:"column:file_id"`
	File         File   `gorm:"foreignKey:FileID"`
	ErrorMessage string `gorm:"column:error_message;type:varchar(255)"`
}

func (DeadLetterQueue) TableName() string {
	return "dead_letter_queue"
}

type SourceFileToDelete struct {
	ID             int32   `gorm:"column:id;primaryKey"`
	FileID         int32   `gorm:"column:file_id"`
	File           File    `gorm:"foreignKey:FileID"`
	SourcePath     string  `gorm:"column:source_path;type:varchar(512)"`
	FailedAttempts int     `gorm:"column:failed_attempts;default:0"`
	LastError      *string `gorm:"column:last_error;type:varchar(255)"`
	CreatedAt      int64   `gorm:"column:created_at;autoCreateTime:unixtime"`
}

func (SourceFileToDelete) TableName() string {
	return "source_files_to_delete"
}
