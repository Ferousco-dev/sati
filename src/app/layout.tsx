import type { Metadata, Viewport } from "next";
// @ts-ignore: CSS module import for side effects
import "./globals.css";
import SmoothScroll from "@/components/SmoothScroll";
import Magnetic from "@/components/Magnetic";
import { StoreProvider } from "@/lib/store";
import CartDrawer from "@/components/CartDrawer";
import { ConsentProvider } from "@/lib/consent";
import { PwaProvider } from "@/lib/pwa";
import { ToastProvider } from "@/components/ui/Toast";

export const metadata: Metadata = {
  title: "SATI-L Yoghurt & Confectionery — Parfait, Cakes, Small Chops & More",
  applicationName: "SATI-L",
  appleWebApp: { capable: true, title: "SATI-L", statusBarStyle: "default" },
  formatDetection: { telephone: false },
  description:
    "SATI-L Yoghurt & Confectionery makes parfait, desserts, cakes, small chops, yoghurt and healthy drinks, with outdoor catering and events (zobo and cocktails). Make you ask for more. Order now.",
};

export const viewport: Viewport = {
  themeColor: "#268adc",
  width: "device-width",
  initialScale: 1,
  viewportFit: "cover",
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
        <ConsentProvider>
          <PwaProvider>
            <ToastProvider>
              <StoreProvider>
                <SmoothScroll />
                <Magnetic />
                {children}
                <CartDrawer />
              </StoreProvider>
            </ToastProvider>
          </PwaProvider>
        </ConsentProvider>
      </body>
    </html>
  );
}
