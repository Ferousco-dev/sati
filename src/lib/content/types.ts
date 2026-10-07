// The shapes served by GET /api/v1/content, /api/v1/store and /api/v1/delivery/zones.

export type Picture = {
  media_id: string;
  alt?: string;
  url?: string;
  width?: number;
  height?: number;
};

export type Link = { label: string; url: string };

export type Hero = {
  tagline: string;
  title_lines: string[];
  cta_label: string;
  cta_url: string;
  images: Picture[];
  mobile_image?: Picture;
};

export type HowItWorks = {
  heading: string;
  steps: { title: string; text: string }[];
};

export type Testimonials = {
  heading: string;
  items: { name: string; role: string; quote: string; rating: number; avatar?: Picture }[];
};

export type Gallery = {
  heading: string;
  images: { image: Picture; caption: string }[];
  instagram_url: string;
};

export type Faq = {
  heading: string;
  items: { question: string; answer: string }[];
};

export type Catering = {
  heading: string;
  text: string;
  banner_image?: Picture;
  banner_note: string;
  cta_label: string;
  cta_url: string;
};

export type Navigation = {
  logo?: Picture;
  logo_alt: string;
  links: Link[];
  order_label: string;
  cart_label: string;
};

export type Showcase = {
  items: { name: string; image: Picture; product_id: string }[];
  order_label: string;
  marquee_text: string;
  marquee_url: string;
};

export type MenuSection = {
  script_lines: string[];
  intro_lines: string[];
  intro_button: string;
  all_label: string;
  add_label: string;
  slide_images: Picture[];
};

export type Footer = {
  tagline: string;
  cta_label: string;
  cta_url: string;
  columns: { title: string; links: Link[] }[];
  legal_text: string;
};

export type Announcement = {
  enabled: boolean;
  text: string;
  link_label: string;
  link_url: string;
};

export type Seo = {
  title: string;
  description: string;
  share_image?: Picture;
};

export type LayoutKey =
  | "hero"
  | "products"
  | "how_it_works"
  | "testimonials"
  | "gallery"
  | "catering"
  | "faq"
  | "hours";

export type Layout = { sections: { key: LayoutKey; visible: boolean }[] };

export type SiteContent = {
  hero: Hero;
  how_it_works: HowItWorks;
  testimonials: Testimonials;
  gallery: Gallery;
  faq: Faq;
  catering: Catering;
  footer: Footer;
  announcement: Announcement;
  seo: Seo;
  layout: Layout;
  navigation: Navigation;
  showcase: Showcase;
  menu_section: MenuSection;
};

export type DayHours = { open: string; close: string } | null;
export type WeekHours = Record<"mon" | "tue" | "wed" | "thu" | "fri" | "sat" | "sun", DayHours>;

export type StoreInfo = {
  name: string;
  address: string;
  phone: string;
  email: string;
  whatsapp_number: string;
  whatsapp_url: string;
  instagram_url: string;
  tiktok_url: string;
  facebook_url: string;
  map_embed_url: string;
  opening_hours: WeekHours;
  timezone: string;
  temporarily_closed: boolean;
  closed_message: string;
};

export type Zone = {
  id: string;
  name: string;
  fee_kobo: number;
};
