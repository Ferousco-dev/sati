import SiteNavbar from "@/components/SiteNavbar";
import { getSiteContent } from "@/lib/content/server";
import OurMenu from "@/components/OurMenu";
import SiteFooter from "@/components/SiteFooter";

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