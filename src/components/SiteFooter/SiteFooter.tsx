import Footer from "@/components/Footer/Footer";
import { getSiteContent, getStoreInfo } from "@/lib/content/server";

export default async function SiteFooter() {
  const [content, store] = await Promise.all([getSiteContent(), getStoreInfo()]);
  return <Footer content={content.footer} store={store} navigation={content.navigation} />;
}
