import SiteNavbar from "@/components/SiteNavbar/SiteNavbar";
import Hero from "@/components/Hero/Hero";
import ProductShowcase from "@/components/ProductShowcase/ProductShowcase";
import Catering from "@/components/Catering/Catering";
import OurMenu from "@/components/OurMenu/OurMenu";
import GalleryStrip from "@/components/GalleryStrip/GalleryStrip";
import SiteFooter from "@/components/SiteFooter/SiteFooter";
import AnnouncementBar from "@/components/AnnouncementBar/AnnouncementBar";
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
