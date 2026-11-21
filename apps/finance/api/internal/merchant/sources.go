package merchant

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Open datasets for the directory. Both are bulk downloads: matching runs
// locally, so no spending data is sent anywhere.
// Overpass servers, tried in order (the main one is often at capacity).
var OverpassURLs = []string{
	"https://overpass-api.de/api/interpreter",
	"https://overpass.private.coffee/api/interpreter",
	"https://maps.mail.ru/osm/tools/overpass/api/interpreter",
}

const (
	acraMetaURL   = "https://api-production.data.gov.sg/v2/public/api/collections/2/metadata"
	acraSearchURL = "https://data.gov.sg/api/action/datastore_search"
	userAgent     = "hub-finance/1.0 (personal finance app; local merchant directory)"
)

const overpassQuery = `[out:json][timeout:180];
area["ISO3166-1"="SG"][admin_level=2]->.sg;
(
  nwr["name"]["amenity"~"^(restaurant|fast_food|cafe|food_court|bar|pub|ice_cream|biergarten|pharmacy|dentist|clinic|doctors|hospital|veterinary|cinema|theatre|fuel|car_wash|marketplace|driving_school)$"](area.sg);
  nwr["name"]["shop"](area.sg);
  nwr["name"]["leisure"~"^(fitness_centre|sports_centre|swimming_pool|bowling_alley|escape_game|amusement_arcade)$"](area.sg);
  nwr["name"]["tourism"~"^(hotel|hostel|attraction|museum|theme_park|zoo|aquarium)$"](area.sg);
  nwr["name"]["healthcare"](area.sg);
);
out tags;`

// FetchOSM downloads Singapore shops, food places and services from
// OpenStreetMap (ODbL) via the Overpass API.
func FetchOSM(ctx context.Context, client *http.Client) ([]Entry, error) {
	var errs []string
	for round := range 2 {
		for _, u := range OverpassURLs {
			out, err := fetchOSM(ctx, client, u)
			if err == nil {
				return out, nil
			}
			errs = append(errs, err.Error())
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(10*(round+1)) * time.Second):
		}
	}
	return nil, fmt.Errorf("all Overpass servers failed: %s", strings.Join(errs, "; "))
}

func fetchOSM(ctx context.Context, client *http.Client, endpoint string) ([]Entry, error) {
	form := url.Values{"data": {overpassQuery}}
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 120))
		host := endpoint
		if pu, err := url.Parse(endpoint); err == nil {
			host = pu.Host
		}
		return nil, fmt.Errorf("%s: %s: %s", host, resp.Status, bytes.TrimSpace(bytes.SplitN(b, []byte("\n"), 2)[0]))
	}
	var body struct {
		Elements []struct {
			Type string            `json:"type"`
			ID   int64             `json:"id"`
			Tags map[string]string `json:"tags"`
		} `json:"elements"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 200<<20)).Decode(&body); err != nil {
		return nil, fmt.Errorf("overpass: %w", err)
	}
	out := make([]Entry, 0, len(body.Elements))
	for _, el := range body.Elements {
		t := el.Tags
		cat, kind := OSMCategory(t)
		if cat == "" || t["name"] == "" {
			continue
		}
		name, branch := t["name"], t["branch"]
		if b := t["brand"]; b != "" && strings.Contains(strings.ToUpper(name), strings.ToUpper(b)) {
			if branch == "" {
				branch = strings.Trim(strings.TrimSpace(strings.Replace(name, b, "", 1)), "-@() ")
			}
			name = b
		}
		addr := strings.Join(nonEmpty(t["addr:housenumber"], t["addr:street"], t["addr:place"], t["addr:postcode"]), " ")
		out = append(out, Entry{Source: "osm", Ref: fmt.Sprintf("%s/%d", el.Type, el.ID), Name: name,
			Branch: branch, Category: cat, Address: addr, Detail: kind})
	}
	return out, nil
}

func nonEmpty(xs ...string) []string {
	var out []string
	for _, x := range xs {
		if x != "" {
			out = append(out, x)
		}
	}
	return out
}

// OSMCategory maps OSM tags to a category, with the tag value as detail.
func OSMCategory(t map[string]string) (string, string) {
	switch a := t["amenity"]; a {
	case "restaurant", "fast_food", "cafe", "food_court", "bar", "pub", "ice_cream", "biergarten":
		return "dining", a
	case "pharmacy", "dentist", "clinic", "doctors", "hospital", "veterinary":
		return "health", a
	case "cinema", "theatre":
		return "entertainment", a
	case "fuel", "car_wash":
		return "car", a
	case "marketplace":
		return "groceries", a
	case "driving_school":
		return "education", a
	}
	if s := t["shop"]; s != "" {
		switch s {
		case "supermarket", "convenience", "greengrocer", "butcher", "seafood", "deli", "frozen_food",
			"health_food", "wine", "alcohol", "beverages", "dairy", "spices":
			return "groceries", s
		case "bakery", "pastry", "confectionery", "coffee", "tea", "chocolate":
			return "dining", s
		case "hairdresser", "beauty", "cosmetics", "perfumery", "massage", "tattoo", "chemist", "laundry", "dry_cleaning":
			return "personal", s
		case "optician", "medical_supply", "hearing_aids":
			return "health", s
		case "furniture", "hardware", "doityourself", "houseware", "interior_decoration", "kitchen", "bed",
			"lighting", "appliance", "electrical", "paint", "curtain", "garden_centre", "bathroom_furnishing":
			return "home", s
		case "car", "car_parts", "car_repair", "tyres", "motorcycle", "fuel":
			return "car", s
		case "travel_agency":
			return "travel", s
		case "florist", "gift":
			return "gifts", s
		case "sports", "outdoor", "bicycle", "fishing":
			return "leisure", s
		case "games", "video_games", "music", "ticket":
			return "entertainment", s
		case "vacant", "yes":
			return "", ""
		}
		return "shopping", s
	}
	switch l := t["leisure"]; l {
	case "fitness_centre", "sports_centre", "swimming_pool", "bowling_alley":
		return "leisure", l
	case "escape_game", "amusement_arcade":
		return "entertainment", l
	}
	switch tr := t["tourism"]; tr {
	case "hotel", "hostel":
		return "travel", tr
	case "attraction", "museum", "theme_park", "zoo", "aquarium":
		return "entertainment", tr
	}
	if h := t["healthcare"]; h != "" {
		return "health", h
	}
	return "", ""
}

// SSICCategory maps a Singapore Standard Industrial Classification code to
// a spending category ("" = not a consumer-facing business).
func SSICCategory(code string) string {
	p := func(s string) bool { return strings.HasPrefix(code, s) }
	switch {
	case p("4711"), p("472"):
		return "groceries"
	case p("473"), p("45"):
		return "car"
	case p("4772"), p("86"), p("75"):
		return "health"
	case p("475"), p("95"):
		return "home"
	case p("47"):
		return "shopping"
	case p("56"), p("107"):
		return "dining"
	case p("55"), p("79"), p("50"), p("51"):
		return "travel"
	case p("4932"):
		return "rides"
	case p("49"):
		return "transport"
	case p("5221"):
		return "car"
	case p("61"), p("35"), p("36"):
		return "bills"
	case p("65"):
		return "insurance"
	case p("85"):
		return "education"
	case p("93"):
		return "leisure"
	case p("59"), p("90"), p("91"), p("92"):
		return "entertainment"
	case p("96"):
		return "personal"
	}
	return ""
}

// FetchACRA downloads live entities registered with ACRA (data.gov.sg,
// Singapore Open Data Licence) whose primary SSIC is consumer-facing. It
// pages the datastore API with a server-side status filter ("Live" for sole
// proprietors and partnerships, "Live Company" for companies), so only a
// fraction of the 2M+ records is transferred.
func FetchACRA(ctx context.Context, client *http.Client, progress func(done, total int)) ([]Entry, error) {
	var meta struct {
		Data struct {
			CollectionMetadata struct {
				ChildDatasets []string `json:"childDatasets"`
			} `json:"collectionMetadata"`
		} `json:"data"`
	}
	if err := getJSON(ctx, client, acraMetaURL, &meta); err != nil {
		return nil, fmt.Errorf("acra metadata: %w", err)
	}
	ids := meta.Data.CollectionMetadata.ChildDatasets
	if len(ids) == 0 {
		return nil, fmt.Errorf("acra metadata: no datasets")
	}
	var out []Entry
	for i, id := range ids {
		if progress != nil {
			progress(i, len(ids))
		}
		offset := 0
		for {
			q := url.Values{
				"resource_id": {id},
				// Sole proprietors are "Live", companies "Live Company".
				"filters": {`{"entity_status_description":["Live","Live Company"]}`},
				"fields":  {"uen,entity_name,primary_ssic_code,primary_ssic_description,block,street_name,building_name,postal_code"},
				"limit":   {"10000"},
				"offset":  {strconv.Itoa(offset)},
			}
			var page struct {
				Success bool `json:"success"`
				Result  struct {
					Total   int                 `json:"total"`
					Records []map[string]string `json:"records"`
				} `json:"result"`
			}
			if err := getJSON(ctx, client, acraSearchURL+"?"+q.Encode(), &page); err != nil {
				return nil, fmt.Errorf("acra %s: %w", id, err)
			}
			for _, r := range page.Result.Records {
				cat := SSICCategory(r["primary_ssic_code"])
				if cat == "" || r["entity_name"] == "" {
					continue
				}
				addr := strings.Join(nonEmpty(na(r["block"]), na(r["street_name"]), na(r["building_name"]), na(r["postal_code"])), " ")
				out = append(out, Entry{Source: "acra", Ref: r["uen"], Name: r["entity_name"], Category: cat,
					Address: addr, Detail: na(r["primary_ssic_description"])})
			}
			offset += len(page.Result.Records)
			if len(page.Result.Records) == 0 || offset >= page.Result.Total {
				break
			}
		}
	}
	if progress != nil {
		progress(len(ids), len(ids))
	}
	return out, nil
}

func na(s string) string {
	if strings.EqualFold(s, "na") {
		return ""
	}
	return s
}

// getJSON fetches with retries on 429/5xx (data.gov.sg rate-limits).
func getJSON(ctx context.Context, client *http.Client, u string, v any) error {
	var last error
	for attempt := range 6 {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt*attempt) * 2 * time.Second):
			}
		}
		req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
		if err != nil {
			return err
		}
		req.Header.Set("User-Agent", userAgent)
		req.Header.Set("Accept", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			last = err
			continue
		}
		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			resp.Body.Close()
			last = fmt.Errorf("%s", resp.Status)
			continue
		}
		if resp.StatusCode != 200 {
			resp.Body.Close()
			return fmt.Errorf("%s", resp.Status)
		}
		err = json.NewDecoder(io.LimitReader(resp.Body, 100<<20)).Decode(v)
		resp.Body.Close()
		return err
	}
	return last
}
