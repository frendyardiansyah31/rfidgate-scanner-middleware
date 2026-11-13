package biblio

// REST API client — diganti dengan direct MySQL query via MySQLFetcher.
// Dipertahankan sebagai referensi jika suatu saat butuh kembali ke REST API.

// import (
// 	"context"
// 	"encoding/json"
// 	"fmt"
// 	"net/http"
// 	"time"
// )

// const pageLimit = 50

// type SlimsClient struct {
// 	baseURL    string
// 	token      string
// 	httpClient *http.Client
// }

// func NewSlimsClient(baseURL, token string) *SlimsClient {
// 	return &SlimsClient{
// 		baseURL: baseURL,
// 		token:   token,
// 		httpClient: &http.Client{
// 			Timeout: 30 * time.Second,
// 		},
// 	}
// }

// type biblioPage struct {
// 	Data         []biblioItem `json:"data"`
// 	TotalRecords int          `json:"totalRecords"`
// }

// type biblioItem struct {
// 	BiblioID int    `json:"biblio_id"`
// 	Title    string `json:"title"`
// 	Image    string `json:"image"`
// }

// type itemPage struct {
// 	Data []itemRecord `json:"data"`
// }

// type itemRecord struct {
// 	ItemCode string `json:"item_code"`
// }

// func (c *SlimsClient) fetchBiblioPage(ctx context.Context, page int) ([]biblioItem, int, error) {
// 	url := fmt.Sprintf("%s/api/v1/bibliography?page=%d&limit=%d", c.baseURL, page, pageLimit)
// 	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
// 	if err != nil {
// 		return nil, 0, err
// 	}
// 	if c.token != "" {
// 		req.Header.Set("Authorization", "Bearer "+c.token)
// 	}
// 	resp, err := c.httpClient.Do(req)
// 	if err != nil {
// 		return nil, 0, err
// 	}
// 	defer resp.Body.Close()
// 	if resp.StatusCode != http.StatusOK {
// 		return nil, 0, fmt.Errorf("bibliography: unexpected status %d", resp.StatusCode)
// 	}
// 	var result biblioPage
// 	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
// 		return nil, 0, err
// 	}
// 	return result.Data, result.TotalRecords, nil
// }

// func (c *SlimsClient) fetchItemsByBiblio(ctx context.Context, biblioID int) ([]string, error) {
// 	url := fmt.Sprintf("%s/api/v1/item?biblio_id=%d", c.baseURL, biblioID)
// 	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
// 	if err != nil {
// 		return nil, err
// 	}
// 	if c.token != "" {
// 		req.Header.Set("Authorization", "Bearer "+c.token)
// 	}
// 	resp, err := c.httpClient.Do(req)
// 	if err != nil {
// 		return nil, err
// 	}
// 	defer resp.Body.Close()
// 	if resp.StatusCode != http.StatusOK {
// 		return nil, fmt.Errorf("item biblio_id=%d: unexpected status %d", biblioID, resp.StatusCode)
// 	}
// 	var result itemPage
// 	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
// 		return nil, err
// 	}
// 	codes := make([]string, 0, len(result.Data))
// 	for _, item := range result.Data {
// 		if item.ItemCode != "" {
// 			codes = append(codes, item.ItemCode)
// 		}
// 	}
// 	return codes, nil
// }
