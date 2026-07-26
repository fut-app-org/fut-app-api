package store

import "time"

type User struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	Email          string     `json:"email"`
	Phone          string     `json:"phone"`
	AvatarColor    string     `json:"avatar_color"`
	Role           string     `json:"role"`
	Status         string     `json:"status"`
	InactiveReason string     `json:"inactive_reason"`
	Delinquent     bool       `json:"delinquent"`
	LastPaymentAt  *time.Time `json:"last_payment_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

type Invite struct {
	ID          string     `json:"id"`
	Token       string     `json:"token"`
	InvitedName string     `json:"invited_name"`
	Role        string     `json:"role"`
	CreatedBy   string     `json:"created_by"`
	CreatorName string     `json:"creator_name"`
	ExpiresAt   time.Time  `json:"expires_at"`
	UsedAt      *time.Time `json:"used_at,omitempty"`
	UsedBy      *string    `json:"used_by,omitempty"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`
	AccessCount int        `json:"access_count"`
	CreatedAt   time.Time  `json:"created_at"`
}

type Match struct {
	ID                   string     `json:"id"`
	MatchDate            string     `json:"match_date"` // YYYY-MM-DD
	StartTime            string     `json:"start_time"` // HH:MM
	EndTime              string     `json:"end_time"`
	Venue                string     `json:"venue"`
	Address              string     `json:"address"`
	ConfirmationDeadline time.Time  `json:"confirmation_deadline"`
	Status               string     `json:"status"`
	CancelReason         string     `json:"cancel_reason"`
	Notes                string     `json:"notes"`
	VotingClosesAt       *time.Time `json:"voting_closes_at,omitempty"`
	FinishedAt           *time.Time `json:"finished_at,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`

	GoingCount      int `json:"going_count"`
	NotGoingCount   int `json:"not_going_count"`
	NoResponseCount int `json:"no_response_count"`
	MediaCount      int `json:"media_count"`
}

type ConfirmationEntry struct {
	UserID      string     `json:"user_id"`
	Name        string     `json:"name"`
	AvatarColor string     `json:"avatar_color"`
	Role        string     `json:"role"`
	Response    string     `json:"response"`
	RespondedAt *time.Time `json:"responded_at,omitempty"`
}

type Team struct {
	ID        string       `json:"id"`
	MatchID   string       `json:"match_id"`
	TeamName  string       `json:"team_name"`
	TeamColor string       `json:"team_color"`
	Position  int          `json:"position"`
	Members   []TeamMember `json:"members"`
}

type TeamMember struct {
	UserID      string `json:"user_id"`
	Name        string `json:"name"`
	AvatarColor string `json:"avatar_color"`
}

type ChargeBatch struct {
	ID                    string    `json:"id"`
	ReferenceMonth        string    `json:"reference_month"` // YYYY-MM
	TotalAmountCents      int64     `json:"total_amount_cents"`
	UserCount             int       `json:"user_count"`
	IndividualAmountCents int64     `json:"individual_amount_cents"`
	DueDate               string    `json:"due_date"` // YYYY-MM-DD
	GeneratedBy           string    `json:"generated_by"`
	GeneratedByName       string    `json:"generated_by_name"`
	CreatedAt             time.Time `json:"created_at"`
}

type Charge struct {
	ID              string     `json:"id"`
	BatchID         string     `json:"batch_id"`
	UserID          string     `json:"user_id"`
	UserName        string     `json:"user_name"`
	UserRole        string     `json:"user_role"`
	AvatarColor     string     `json:"avatar_color"`
	ReferenceMonth  string     `json:"reference_month"` // YYYY-MM
	AmountCents     int64      `json:"amount_cents"`
	Status          string     `json:"status"`
	DueDate         string     `json:"due_date"` // YYYY-MM-DD
	PaidAt          *time.Time `json:"paid_at,omitempty"`
	PaidMethod      string     `json:"paid_method"`
	RegisteredBy    *string    `json:"registered_by,omitempty"`
	RegisteredName  string     `json:"registered_by_name,omitempty"`
	PixPayload      string     `json:"pix_payload"`
	PixTicketURL    string     `json:"pix_ticket_url"`
	PixQRCodeBase64 string     `json:"pix_qr_code_base64"`
	ProviderOrderID string     `json:"-"`
	CreatedAt       time.Time  `json:"created_at"`
}

type VoteResult struct {
	Category  string   `json:"category"`
	VoteCount int      `json:"vote_count"`
	Winners   []Winner `json:"winners"`
}

type Winner struct {
	UserID      string `json:"user_id"`
	Name        string `json:"name"`
	AvatarColor string `json:"avatar_color"`
	Votes       int    `json:"votes"`
}

type Media struct {
	ID           string    `json:"id"`
	MatchID      string    `json:"match_id"`
	UploadedBy   string    `json:"uploaded_by"`
	UploaderName string    `json:"uploader_name"`
	Type         string    `json:"type"`
	URL          string    `json:"url"`
	ThumbnailURL string    `json:"thumbnail_url"`
	Caption      string    `json:"caption"`
	CreatedAt    time.Time `json:"created_at"`
}

type Activity struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}
