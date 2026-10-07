import type { MenuItem } from "./store";

export type MenuCategory = {
  id: string;
  name: string;
  items: MenuItem[];
};

// Single source of truth for the menu. Prices are in Naira.
export const MENU: MenuCategory[] = [
  {
    id: "pizza",
    name: "Pizza & Grills",
    items: [
      {
        id: "pizza-pepperoni",
        name: "Loaded Pepperoni Pizza",
        price: 6500,
        image: "/images/products/pizza.webp",
        category: "Pizza & Grills",
        tags: ["Popular", "Spicy"],
      },
      {
        id: "beast-fuel",
        name: "Beast Fuel Burger",
        price: 5200,
        image: "/images/products/beast-fuel.webp",
        category: "Pizza & Grills",
        tags: ["Chef's pick"],
      },
    ],
  },
  {
    id: "chicken",
    name: "Chicken",
    items: [
      {
        id: "crispy-chick",
        name: "Crispy Chicken Bucket",
        price: 4800,
        image: "/images/products/crispy-chick.webp",
        category: "Chicken",
        tags: ["Popular"],
      },
      {
        id: "chicken-classic",
        name: "Grilled Chicken Plate",
        price: 4200,
        image: "/images/products/chicken.webp",
        category: "Chicken",
        tags: ["Healthy"],
      },
    ],
  },
  {
    id: "shawarma",
    name: "Shawarma",
    items: [
      {
        id: "shawarma-double",
        name: "Double Meat Shawarma",
        price: 3500,
        image: "/images/products/shawarma.webp",
        category: "Shawarma",
        tags: ["Popular", "Spicy"],
      },
    ],
  },
  {
    id: "snacks",
    name: "Snacks & Sides",
    items: [
      {
        id: "yogurt-chicken-chips",
        name: "Chicken & Chips Combo",
        price: 3800,
        image: "/images/products/yogurt-chicken-chips.webp",
        category: "Snacks & Sides",
        tags: ["Combo"],
      },
      {
        id: "hot-dog",
        name: "Loaded Hot Dog",
        price: 2900,
        image: "/images/products/hot-dog.webp",
        category: "Snacks & Sides",
      },
      {
        id: "the-snag",
        name: "The Snag Sausage",
        price: 2500,
        image: "/images/products/the-snag.webp",
        category: "Snacks & Sides",
      },
    ],
  },
  {
    id: "cake",
    name: "Cake & Sweets",
    items: [
      {
        id: "cake-slice",
        name: "Loaded Cake Slice",
        price: 2200,
        image: "/images/products/cake.webp",
        category: "Cake & Sweets",
        tags: ["Sweet"],
      },
    ],
  },
];

// Flat lookup of every item by id, for hydrating from saved orders etc.
export const MENU_ITEMS: MenuItem[] = MENU.flatMap((c) => c.items);

// Cheap add-ons surfaced as upsells at checkout to grow the basket.
export const ADDONS: MenuItem[] = [
  {
    id: "addon-fries",
    name: "Crispy Fries",
    price: 1200,
    image: "/images/products/yogurt-chicken-chips.webp",
    category: "Add-ons",
  },
  {
    id: "addon-snag",
    name: "Extra Sausage",
    price: 1000,
    image: "/images/products/the-snag.webp",
    category: "Add-ons",
  },
  {
    id: "addon-cake",
    name: "Cake Slice",
    price: 2200,
    image: "/images/products/cake.webp",
    category: "Add-ons",
  },
];

export const naira = (n: number) =>
  "₦" + n.toLocaleString("en-NG", { maximumFractionDigits: 0 });
