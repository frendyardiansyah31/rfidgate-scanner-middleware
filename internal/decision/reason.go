package decision

type Reason string

const (
	ReasonLoanValid     Reason = "LOAN_VALID"
	ReasonNotLoan       Reason = "NOT_LOAN"
	ReasonUnknownRFID   Reason = "UNKNOWN_RFID"
	ReasonTimeout       Reason = "TIMEOUT"
	ReasonCacheError    Reason = "CACHE_ERROR"
	ReasonSQLiteError   Reason = "SQLITE_ERROR"
	ReasonInternalError Reason = "INTERNAL_ERROR"
	ReasonInvalidInput  Reason = "INVALID_INPUT"
)
