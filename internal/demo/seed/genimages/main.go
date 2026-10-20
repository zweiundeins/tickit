// Command genimages draws tickit's image library (FR-84): diagrams,
// screenshots of the fictional shop and pixel-art photos, as SVG files, and
// writes their index. The output is the same on every run.
//
//	go run ./internal/demo/seed/genimages -out internal/web/static/img/lib -index internal/demo/seed/data/images.json
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
)

type image struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Alt    string `json:"alt"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Kind   string `json:"kind"`
}

type entry struct {
	id, kind, title, alt string
	draw                 func(title string) string
}

func ph(f func() *canvas) func(string) string {
	return func(title string) string { return photo(f(), 5, title) }
}

var library = []entry{
	{"architecture-overview", "diagram", "Architecture overview", "Boxes and arrows: the web shop and the app reach summit through the edge; summit uses PostgreSQL and cairn; porter talks to the warehouse and the mail relay; Paygrove sends webhooks.", diagramArchitecture},
	{"checkout-sequence", "diagram", "Checkout sequence", "Sequence diagram of a payment between the customer, the web shop, summit and Paygrove, ending with the payment webhook.", diagramCheckoutSequence},
	{"stock-sync-flow", "diagram", "Stock sync with its guard", "Flow from the warehouse's stock file through porter and a new guard into PostgreSQL and cairn; a failed guard stops the sync and pages on-call.", diagramStockSync},
	{"data-model", "diagram", "Core data model", "Entity diagram of products, variants, stock levels, customers, orders and order lines with their relations.", diagramDataModel},
	{"deploy-pipeline", "diagram", "Deploy pipeline", "Seven steps from a merge to production: test, build, staging, smoke tests, a canary at 10 percent, then everyone.", diagramDeployPipeline},
	{"incident-response", "diagram", "Incident response", "Flowchart: an alert, acknowledgement, a decision whether customers are affected, then an incident with updates and a fix, and a postmortem.", diagramIncident},
	{"search-indexing", "diagram", "Search indexing", "summit writes changes to an outbox; the cairn indexer reads it and updates the index that the shop and the app query.", diagramSearchIndexing},
	{"app-release-train", "diagram", "App release train", "A two-week timeline: features on main, branch cut and beta, store review, then a staged rollout.", diagramReleaseTrain},
	{"team-map", "diagram", "Team map", "Six pixel avatars grouped into product, engineering, and quality and support, with names and roles.", diagramTeam},
	{"roadmap-board", "diagram", "Roadmap board", "Three columns, now, next and later, with the projects planned in each.", diagramRoadmap},
	{"chart-checkout-conversion", "diagram", "Checkout conversion chart", "Bar chart of weekly checkout conversion: the three-step checkout stays near 2.8 percent, the one-page checkout climbs from 3.1 to 3.6 percent.", chartConversion},
	{"chart-api-latency", "diagram", "API latency chart", "Line chart of p50 and p95 latency over 30 days, with a spike during the stock sync incident and a drop after search moved to cairn.", chartLatency},
	{"chart-crash-free", "diagram", "Crash-free sessions chart", "Bar chart of crash-free sessions per app version; the first 3.0 beta dips to 98.2 percent.", chartCrashFree},
	{"chart-returns", "diagram", "Return reasons chart", "Horizontal bars of return reasons; too small and too big together cause more than half of all returns.", chartReturns},
	{"shop-home", "screenshot", "Web shop home page", "The shop's home page with a mountain banner for the First Snow Sale and four new products.", shotShopHome},
	{"product-page", "screenshot", "Product page", "The product page of the Firn 30 backpack with colours, back lengths and an add to cart button.", shotProduct},
	{"checkout-one-page", "screenshot", "One-page checkout", "The one-page checkout: address, delivery and payment on the left, the order summary with a voucher and the pay button on the right.", shotCheckout},
	{"cart-drawer", "screenshot", "Cart drawer", "The cart drawer open over a category page, with two items, a free delivery hint and the checkout button.", shotCartDrawer},
	{"search-results", "screenshot", "Search results", "Search results for rain jacket with filters for category, size and colour.", shotSearch},
	{"size-guide", "screenshot", "Size guide dialog", "A size guide dialog for hiking boots: a table of EU, UK and US sizes with foot lengths, size 42 highlighted.", shotSizeGuide},
	{"error-page", "screenshot", "Error page", "The shop's error page: pixel mountains in fog and the words Lost in the fog.", shotErrorPage},
	{"app-home", "screenshot", "App home screen", "The app's home screen with the sale banner, the next trip's pack list progress and products back in stock.", shotAppHome},
	{"app-pack-list", "screenshot", "App pack list", "A pack list in the app for a three-day hut trip, grouped into clothing, gear and extras, with two missing items.", shotAppPackList},
	{"app-order-tracking", "screenshot", "App order tracking", "Order tracking in the app: placed, packed, handed to the carrier, out for delivery, and delivered still to come.", shotAppOrder},
	{"back-office-orders", "screenshot", "Back office orders", "The back office list of today's orders with totals and coloured status badges.", shotBackOffice},
	{"ops-dashboard", "screenshot", "Operations dashboard", "A dark dashboard for summit with request rate, latency, error rate, stock sync status and recent alerts.", shotOpsDashboard},
	{"order-email", "screenshot", "Shipping email", "The shipping confirmation email with a mountain banner, a tracking button and the order's items.", shotOrderEmail},
	{"alpenglow-peaks", "photo", "Alpenglow on the peaks", "Pixel art of snowy peaks glowing pink at dawn above a dark ridge with fir trees.", ph(sceneAlpenglow)},
	{"mountain-hut", "photo", "Mountain hut", "Pixel art of a wooden hut with red shutters on a stone base, smoke from the chimney, a meadow path and peaks behind.", ph(sceneHut)},
	{"trail-signpost", "photo", "Trail signpost", "Pixel art of a yellow hiking signpost with three arrows on a summer meadow.", ph(sceneSignpost)},
	{"lake-reflection", "photo", "Mountain lake", "Pixel art of peaks and a forest mirrored in a still mountain lake.", ph(sceneLake)},
	{"tent-at-night", "photo", "Tent at night", "Pixel art of a lit tent under a starry sky with a crescent moon and dark mountains.", ph(sceneTent)},
	{"hiking-boots", "photo", "Hiking boots", "Pixel art product photo of a pair of brown leather hiking boots with yellow laces.", ph(sceneBoots)},
	{"backpack", "photo", "Firn 30 backpack", "Pixel art product photo of a teal day pack with a front pocket and black straps.", ph(sceneBackpack)},
	{"glacier", "photo", "Rope team on a glacier", "Pixel art of three climbers roped together crossing a glacier with crevasses below peaks.", ph(sceneGlacier)},
	{"cable-car", "photo", "Cable car", "Pixel art of a red cable car cabin above a forested valley.", ph(sceneCableCar)},
	{"cows-meadow", "photo", "Cows on an alpine meadow", "Pixel art of brown and white cows with bells on a flowery meadow behind a fence.", ph(sceneCows)},
	{"rain-jacket", "photo", "Nebel rain shell", "Pixel art product photo of a rust-coloured rain jacket on a hanger.", ph(sceneJacket)},
	{"campfire", "photo", "Campfire at dusk", "Pixel art of a campfire with logs and sparks at dusk, a mug beside it.", ph(sceneCampfire)},
	{"warehouse", "photo", "Warehouse aisle", "Pixel art of the warehouse: blue shelves full of cardboard boxes and a teal crate on a pallet truck.", ph(sceneWarehouse)},
}

var sizeRe = regexp.MustCompile(`^<svg [^>]*width="(\d+)" height="(\d+)"`)

func main() {
	out := flag.String("out", "internal/web/static/img/lib", "directory for the SVG files")
	index := flag.String("index", "internal/demo/seed/data/images.json", "the library's index")
	flag.Parse()
	if err := os.MkdirAll(*out, 0o755); err != nil {
		log.Fatal(err)
	}
	var imgs []image
	total := 0
	for _, e := range library {
		svg := e.draw(e.title)
		m := sizeRe.FindStringSubmatch(svg)
		if m == nil {
			log.Fatalf("%s: no size", e.id)
		}
		var w, h int
		fmt.Sscan(m[1], &w)
		fmt.Sscan(m[2], &h)
		if err := os.WriteFile(filepath.Join(*out, e.id+".svg"), []byte(svg), 0o644); err != nil {
			log.Fatal(err)
		}
		total += len(svg)
		imgs = append(imgs, image{e.id, e.title, e.alt, w, h, e.kind})
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(imgs); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*index, buf.Bytes(), 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%d images, %d bytes\n", len(imgs), total)
}
