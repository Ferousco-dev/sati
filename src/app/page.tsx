import Navbar from "@/components/Navbar";
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
      <Navbar />
      <AnnouncementBar content={content.announcement} />
      <Hero content={content.hero} />
      <ProductShowcase />
      <Catering content={content.catering} store={store} />
      <OurMenu />
      <GalleryStrip content={content.gallery} />
      <SiteFooter />
    </>
  );
}
