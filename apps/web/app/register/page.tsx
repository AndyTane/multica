"use client";

import { ProductMark } from "@multica/ui/components/common/product-mark";
import { PRODUCT_NAME } from "@multica/core/config/brand";
import { RegistrationPage } from "@multica/views/auth";

export default function Page() {
  return <RegistrationPage logo={<ProductMark name={PRODUCT_NAME} size={32} />} />;
}
