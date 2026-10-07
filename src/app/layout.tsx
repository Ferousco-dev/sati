import type { Metadata } from "next";
// @ts-ignore: CSS module import for side effects
import "./globals.css";
import SmoothScroll from "@/components/SmoothScroll";
import Magnetic from "@/components/Magnetic";
import { StoreProvider } from "@/lib/store";
import CartDrawer from "@/components/CartDrawer";

export const metadata: Metadata = {
  title: "SATI-L Yoghurt & Confectionery — Parfait, Cakes, Small Chops & More",
  description:
    "SATI-L Yoghurt & Confectionery makes parfait, desserts, cakes, small chops, yoghurt and healthy drinks, with outdoor catering and events (zobo and cocktails). Make you ask for more. Order now.",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en">
      <head>
        <link rel="preconnect" href="https://fonts.googleapis.com" />
        <link
          rel="preconnect"
          href="https://fonts.gstatic.com"
          crossOrigin="anonymous"
        />
        <link
          href="https://fonts.googleapis.com/css2?family=Instrument+Sans:wght@500;600;700&family=Permanent+Marker&display=swap"
          rel="stylesheet"
        />
      </head>
      <body>
        <StoreProvider>
          <SmoothScroll />
          <Magnetic />
          {children}
          <CartDrawer />
        </StoreProvider>
      </body>
    </html>
  );
}
