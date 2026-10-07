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
import WhatsAppButton from "@/components/WhatsAppButton";
import OrderFab from "@/components/OrderFab";
import ConsentBar from "@/components/ConsentBar";
import { getSiteContent, getStoreInfo } from "@/lib/content/server";

export async function generateMetadata(): Promise<Metadata> {
  const { seo } = await getSiteContent();
  return {
    title: seo.title,
    description: seo.description,
    applicationName: "SATI-L",
    appleWebApp: { capable: true, title: "SATI-L", statusBarStyle: "default" },
    formatDetection: { telephone: false },
    openGraph: {
      title: seo.title,
      description: seo.description,
      type: "website",
      images: seo.share_image?.url ? [{ url: seo.share_image.url }] : undefined,
    },
  };
}

export const viewport: Viewport = {
  themeColor: "#268adc",
  width: "device-width",
  initialScale: 1,
  viewportFit: "cover",
};

export default async function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const store = await getStoreInfo();

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
      <body suppressHydrationWarning>
        <ConsentProvider>
          <PwaProvider>
            <ToastProvider>
              <StoreProvider>
                <SmoothScroll />
                <Magnetic />
                {children}
                <CartDrawer />
                <WhatsAppButton number={store.whatsapp_number} />
                <OrderFab />
                <ConsentBar />
              </StoreProvider>
            </ToastProvider>
          </PwaProvider>
        </ConsentProvider>
      </body>
    </html>
  );
}
