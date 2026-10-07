import Navbar from "@/components/Navbar";
import { getSiteContent } from "@/lib/content/server";

export default async function SiteNavbar() {
  const content = await getSiteContent();
  return <Navbar content={content.navigation} />;
}
