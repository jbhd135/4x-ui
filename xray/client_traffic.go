package xray

// ClientTraffic represents traffic statistics and limits for a specific client.
// It tracks upload/download usage, expiry times, and online status for inbound clients.
type ClientTraffic struct {
	Id         int    `json:"id" form:"id" gorm:"primaryKey;autoIncrement"`
	InboundId  int    `json:"inboundId" form:"inboundId"`
	Enable     bool   `json:"enable" form:"enable"`
	Email      string `json:"email" form:"email" gorm:"unique"`
	UUID       string `json:"uuid" form:"uuid" gorm:"-"`
	SubId      string `json:"subId" form:"subId" gorm:"-"`
	Up         int64  `json:"up" form:"up"`
	Down       int64  `json:"down" form:"down"`
	AllTime    int64  `json:"allTime" form:"allTime"`
	ExpiryTime int64  `json:"expiryTime" form:"expiryTime"`
	Total      int64  `json:"total" form:"total"`
	Reset      int    `json:"reset" form:"reset" gorm:"default:0"`
	LastOnline int64  `json:"lastOnline" form:"lastOnline" gorm:"default:0"`
	// DailyTrafficLimit is this client's daily upload plus download allowance.
	// Zero means unlimited.
	DailyTrafficLimit int64 `json:"dailyTrafficLimit" form:"dailyTrafficLimit" gorm:"column:daily_traffic_limit;default:10737418240"`
	// TodayTraffic is populated from DailyClientTraffic when returning clients.
	TodayTraffic int64 `json:"todayTraffic" gorm:"-"`
	// DailyBlockedDate is set when the client is automatically disabled by the
	// daily traffic limit. It lets the midnight reset distinguish this state
	// from a manually disabled client.
	DailyBlockedDate string `json:"dailyBlockedDate" gorm:"column:daily_blocked_date;default:''"`
	// DailyOverrideLimit is the byte threshold granted by a manual re-enable
	// for the current day. Zero means the normal daily limit applies.
	DailyOverrideLimit int64 `json:"dailyOverrideLimit" gorm:"column:daily_override_limit;default:0"`
	// DailyOverrideDate scopes a manual override to one panel-local day. This
	// prevents a temporary allowance from carrying over after midnight.
	DailyOverrideDate string `json:"dailyOverrideDate" gorm:"column:daily_override_date;default:''"`
}
