import SiteNavbar from "@/components/SiteNavbar/SiteNavbar";
import { getSiteContent } from "@/lib/content/server";
import OurMenu from "@/components/OurMenu/OurMenu";
import SiteFooter from "@/components/SiteFooter/SiteFooter";

export const revalidate = 60;

export default async function menu(){
  const content = await getSiteContent();
  return(
    <>
    <SiteNavbar />
    <OurMenu content={content.menu_section} showFullMenuLink={false} showSearch />
    <SiteFooter />
    </>
  )
}