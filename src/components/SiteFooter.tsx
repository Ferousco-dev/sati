import Footer from "@/components/Footer";
import { getSiteContent, getStoreInfo, getZones } from "@/lib/content/server";

/** The footer with its text, hours and contact details loaded from the API. */
export default async function SiteFooter() {
  const [content, store, zones] = await Promise.all([getSiteContent(), getStoreInfo(), getZones()]);
  return <Footer content={content.footer} store={store} zones={zones} />;
}
