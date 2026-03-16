from pydantic import BaseModel


class ScrapedProduct(BaseModel):
    name: str
    brand: str | None = None
    ean: str | None = None
    unit: str = "piece"
    unit_quantity: float | None = None
    image_url: str | None = None
    price: float
    currency: str = "CHF"
    external_id: str | None = None
    product_url: str | None = None
