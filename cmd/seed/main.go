package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Ferousco-dev/sati/backend/pkg/config"
	"github.com/Ferousco-dev/sati/backend/pkg/database"
)

type seedProduct struct {
	category    string
	name        string
	description string
	priceKobo   int64
}

var categoryNames = []string{"Yoghurt", "Parfait", "Drinks"}

var seedProducts = []seedProduct{
	{"Yoghurt", "Plain Yoghurt", "Creamy natural yoghurt.", 250000},
	{"Yoghurt", "Strawberry Yoghurt", "Yoghurt blended with strawberries.", 300000},
	{"Parfait", "Fruit Parfait", "Layers of yoghurt, fruit and granola.", 450000},
	{"Drinks", "Drinking Yoghurt", "Chilled drinking yoghurt.", 200000},
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.IsProduction() {
		return fmt.Errorf("refusing to seed in production")
	}

	ctx := context.Background()
	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	for i, name := range categoryNames {
		if _, err := pool.Exec(ctx,
			`INSERT INTO categories (name, sort_order) VALUES ($1, $2) ON CONFLICT (name) DO NOTHING`, name, i); err != nil {
			return err
		}
	}

	var existing int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM products`).Scan(&existing); err != nil {
		return err
	}
	if existing > 0 {
		fmt.Println("products already exist, skipping")
		return nil
	}

	for _, p := range seedProducts {
		if _, err := pool.Exec(ctx,
			`INSERT INTO products (category_id, name, description, price_kobo)
			 SELECT id, $2, $3, $4 FROM categories WHERE name = $1`,
			p.category, p.name, p.description, p.priceKobo); err != nil {
			return err
		}
	}
	fmt.Println("seeded", len(categoryNames), "categories and", len(seedProducts), "products")
	return nil
}
