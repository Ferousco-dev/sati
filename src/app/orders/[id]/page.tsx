import { Suspense } from "react";
import SiteNavbar from "@/components/SiteNavbar/SiteNavbar";
import SiteFooter from "@/components/SiteFooter/SiteFooter";
import OrderStatusView from "./OrderStatusView";

export default function OrderDetailPage() {
  return (
    <>
      <SiteNavbar />
      <Suspense>
        <OrderStatusView />
      </Suspense>
      <SiteFooter />
    </>
  );
}
