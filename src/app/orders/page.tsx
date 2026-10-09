import SiteNavbar from "@/components/SiteNavbar/SiteNavbar";
import SiteFooter from "@/components/SiteFooter/SiteFooter";
import OrdersView from "./OrdersView";

export default function OrdersPage() {
  return (
    <>
      <SiteNavbar />
      <OrdersView />
      <SiteFooter />
    </>
  );
}
