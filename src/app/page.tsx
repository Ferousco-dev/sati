import SiteNavbar from "@/components/SiteNavbar";
import Hero from "@/components/Hero";
import ProductShowcase from "@/components/ProductShowcase";
import Catering from "@/components/Catering";
import OurMenu from "@/components/OurMenu";
import GalleryStrip from "@/components/GalleryStrip";
import SiteFooter from "@/components/SiteFooter";
import AnnouncementBar from "@/components/AnnouncementBar";
import { getSiteContent, getStoreInfo } from "@/lib/content/server";

// Rebuilt in the background every minute so edits made in the admin appear without a deploy.
export const revalidate = 60;

export default async function Home() {
  const [content, store] = await Promise.all([getSiteContent(), getStoreInfo()]);

  return (
    <>
      <SiteNavbar />
      <AnnouncementBar content={content.announcement} />
      <Hero content={content.hero} />
      <ProductShowcase content={content.showcase} />
      <Catering content={content.catering} store={store} />
      <OurMenu content={content.menu_section} />
      <GalleryStrip content={content.gallery} />
      <SiteFooter />
    </>
  );
}
