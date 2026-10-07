"use client";

import { useState } from "react";
import Navbar from "@/components/Navbar";
import OurMenu from "@/components/OurMenu";
import Footer from "@/components/Footer";

export default function menu(){
  return(
    <>
    <Navbar />
    <OurMenu />
    <Footer />
    </>
  )
}