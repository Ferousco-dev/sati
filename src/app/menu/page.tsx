import Navbar from "@/components/Navbar";
import OurMenu from "@/components/OurMenu";
import SiteFooter from "@/components/SiteFooter";

export default function menu(){
  return(
    <>
    <Navbar />
    <OurMenu showFullMenuLink={false} />
    <SiteFooter />
    </>
  )
}