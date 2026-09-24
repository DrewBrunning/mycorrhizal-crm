package models

// GiftShoppingItem is one row of the /occasion-obligations/gift-shopping-list
// aggregate (ADR 0024, issue #387, ticket #1226) — an active Kind="gift"
// obligation due within the window, joined against Gift to show whether
// this cycle's gift already has a linked idea/purchase.
type GiftShoppingItem struct {
	ContactID    uint   `json:"contact_id"`
	ContactName  string `json:"contact_name"`
	ObligationID string `json:"obligation_id"`
	Label        string `json:"label"`
	Date         string `json:"date"`
	DaysUntil    int    `json:"days_until"`
	// Status ∈ needed|idea|purchased|given|received (the last four mirror
	// Gift.Status* exactly; "needed" means no matching Gift row was found).
	Status       string `json:"status"`
	LinkedGiftID string `json:"linked_gift_id,omitempty"`
}
