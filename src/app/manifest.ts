import type { MetadataRoute } from "next";

export default function manifest(): MetadataRoute.Manifest {
  return {
    id: "/menu",
    name: "SATI-L Yoghurt & Confectionery",
    short_name: "SATI-L",
    description:
      "Order parfait, cakes, small chops, yoghurt and healthy drinks. Make you ask for more.",
    // The installed app skips the landing page and opens on the menu.
    start_url: "/menu?source=pwa",
    scope: "/",
    display: "standalone",
    orientation: "portrait",
    background_color: "#f8fafc",
    theme_color: "#268adc",
    categories: ["food", "shopping"],
    icons: [
      { src: "/icons/icon-192.png", sizes: "192x192", type: "image/png", purpose: "any" },
      { src: "/icons/icon-512.png", sizes: "512x512", type: "image/png", purpose: "any" },
      { src: "/icons/icon-maskable-512.png", sizes: "512x512", type: "image/png", purpose: "maskable" },
    ],
    shortcuts: [
      { name: "Menu", url: "/menu?source=shortcut" },
      { name: "My orders", url: "/account?source=shortcut" },
    ],
  };
}
