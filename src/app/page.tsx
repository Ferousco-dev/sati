import Navbar from "@/components/Navbar";
import Hero from "@/components/Hero";
import ProductShowcase from "@/components/ProductShowcase";
import Catering from "@/components/Catering";
import OurMenu from "@/components/OurMenu";
import Footer from "@/components/Footer";

export default function Home() {
  return (
    <>
      <Navbar />
      <Hero />
      <ProductShowcase />
      <Catering />
      <OurMenu />
      <Footer />
    </>
  );
}
