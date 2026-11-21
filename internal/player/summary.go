package player

import "context"

// Counts describes distinct (network_id, player_name) records. A whitelist-only
// entry is known even before its first join; it is not a social-account binding.
type Counts struct {
	Players       int64 `json:"players"`
	Online        int64 `json:"online"`
	Bound         int64 `json:"bound"`
	WhitelistOnly int64 `json:"whitelist_only"`
	Unlisted      int64 `json:"unlisted"`
}

func (s *Service) Summary(ctx context.Context) (map[string]Counts, error) {
	// One statement observes one SQLite snapshot and uses the existing composite
	// primary keys for both joins. Do not fetch a page and call its length a total.
	rows, err := s.db.QueryContext(ctx, `
WITH identities AS (
    SELECT network_id,player_name FROM player_stats
    UNION SELECT network_id,player_name FROM bindings
)
SELECT i.network_id,COUNT(*),
    SUM(CASE WHEN p.session_started_at IS NOT NULL THEN 1 ELSE 0 END),
    SUM(CASE WHEN b.user_id <> '' THEN 1 ELSE 0 END),
    SUM(CASE WHEN b.user_id = '' THEN 1 ELSE 0 END),
    SUM(CASE WHEN b.player_name IS NULL THEN 1 ELSE 0 END)
FROM identities i
LEFT JOIN player_stats p ON p.network_id=i.network_id AND p.player_name=i.player_name
LEFT JOIN bindings b ON b.network_id=i.network_id AND b.player_name=i.player_name
GROUP BY i.network_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]Counts{}
	for rows.Next() {
		var network string
		var count Counts
		if err := rows.Scan(&network, &count.Players, &count.Online, &count.Bound, &count.WhitelistOnly, &count.Unlisted); err != nil {
			return nil, err
		}
		result[network] = count
	}
	return result, rows.Err()
}
