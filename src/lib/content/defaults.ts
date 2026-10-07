import type { SiteContent, StoreInfo } from "./types";

// Shown when the API cannot be reached, and for any section nobody has published yet.
// These match the defaults built into the backend.
export const defaultContent: SiteContent = {
  hero: {
    tagline: "Make you ask for more.",
    title_lines: ["Parfait, Cakes,", "Small chops, yoghurt & more."],
    cta_label: "View menu",
    cta_url: "/menu",
    images: [],
  },
  how_it_works: {
    heading: "How it works",
    steps: [
      { title: "Choose", text: "Pick your favourites from the menu." },
      { title: "Pay", text: "Pay online, by bank transfer, or when it arrives." },
      { title: "We prepare", text: "Everything is made fresh to order." },
      { title: "Delivered", text: "Collect it, or we bring it to you." },
    ],
  },
  testimonials: { heading: "What people say", items: [] },
  gallery: { heading: "From our kitchen", images: [], instagram_url: "" },
  faq: {
    heading: "Questions",
    items: [
      {
        question: "How can I pay?",
        answer:
          "Pay online with your card, send a bank transfer, or pay when you collect or when your order arrives. You choose at checkout.",
      },
      {
        question: "Do you deliver?",
        answer:
          "Yes. Choose delivery at checkout and pick your area. The delivery areas and their fees are listed on this page.",
      },
      {
        question: "Can I order a cake or catering for an event?",
        answer:
          "Message us on WhatsApp with your date and what you need, and we will confirm if we can make it.",
      },
    ],
  },
  catering: {
    heading: "We've got your catering covered",
    text: "From weddings and birthdays to cocktail evenings and outdoor events, our small chops, parfait, cakes, zobo and cocktails are made to feed any crowd.",
    banner_note: "Make them ask for more",
    cta_label: "Learn more",
    cta_url: "#catering",
  },
  footer: {
    tagline: "Make you ask for more",
    cta_label: "Order Now",
    cta_url: "/menu",
    columns: [
      {
        title: "Menu",
        links: [
          { label: "Parfait", url: "/menu#parfait" },
          { label: "Desserts & Cakes", url: "/menu#cakes" },
          { label: "Small Chops", url: "/menu#small-chops" },
          { label: "Yoghurt", url: "/menu#yoghurt" },
          { label: "Healthy Drinks", url: "/menu#drinks" },
        ],
      },
      {
        title: "SATI-L",
        links: [
          { label: "Our Story", url: "/brand" },
          { label: "Find a Spot", url: "/stores" },
          { label: "Catering", url: "/catering" },
          { label: "Careers", url: "/careers" },
        ],
      },
      {
        title: "Visit",
        links: [
          { label: "Visit our store", url: "/stores" },
          { label: "Open 9am – 11pm daily", url: "/stores" },
          { label: "hello@orentegrills.com", url: "mailto:hello@orentegrills.com" },
        ],
      },
    ],
    legal_text: "",
  },
  navigation: {
    logo_alt: "SATI-L Yoghurt & Confectionery",
    links: [
      { label: "Our Brand", url: "/#brand" },
      { label: "Stores", url: "/#stores" },
      { label: "Our Menu", url: "/menu" },
      { label: "My Orders", url: "/account" },
    ],
    order_label: "Order Now",
    cart_label: "Cart",
  },
  // Empty until the admin chooses pictures. The page then uses the built-in cards in src/lib/fallbacks.ts.
  showcase: { items: [], order_label: "Order Now", marquee_text: "Order Now", marquee_url: "/menu" },
  menu_section: {
    script_lines: ["Sweet treats", "& healthy sips"],
    intro_lines: ["Fresh & sweet", "for every occasion"],
    intro_button: "See the full menu",
    all_label: "All",
    add_label: "Add to bag",
    slide_images: [],
  },
  announcement: { enabled: false, text: "", link_label: "", link_url: "" },
  seo: {
    title: "SATI-L Yoghurt & Confectionery — Parfait, Cakes, Small Chops & More",
    description:
      "Parfait, desserts, cakes, small chops, yoghurt and healthy drinks, with outdoor catering and events. Make you ask for more.",
  },
  layout: {
    sections: [
      { key: "hero", visible: true },
      { key: "products", visible: true },
      { key: "how_it_works", visible: true },
      { key: "testimonials", visible: true },
      { key: "gallery", visible: true },
      { key: "catering", visible: true },
      { key: "faq", visible: true },
      { key: "hours", visible: true },
    ],
  },
};

export const defaultStore: StoreInfo = {
  name: "SATI-L Yoghurt & Confectionery",
  address: "",
  phone: "",
  email: "",
  whatsapp_number: "",
  whatsapp_url: "",
  instagram_url: "",
  tiktok_url: "",
  facebook_url: "",
  map_embed_url: "",
  opening_hours: { mon: null, tue: null, wed: null, thu: null, fri: null, sat: null, sun: null },
  timezone: "Africa/Lagos",
  temporarily_closed: false,
  closed_message: "",
};
