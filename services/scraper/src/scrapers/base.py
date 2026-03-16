import abc
import logging
import time

import httpx

from src.models.product import ScrapedProduct

logger = logging.getLogger(__name__)


class BaseScraper(abc.ABC):
    """Base class for all store scrapers."""

    store_slug: str
    store_name: str
    base_url: str

    def __init__(self):
        self.client = httpx.Client(
            timeout=30,
            headers={
                "User-Agent": "SwissGroceryBot/1.0 (price-comparison research)",
                "Accept-Language": "de-CH,de;q=0.9,en;q=0.8",
            },
            follow_redirects=True,
        )

    @abc.abstractmethod
    def scrape_category(self, category_url: str) -> list[ScrapedProduct]:
        """Scrape all products from a category page."""
        ...

    @abc.abstractmethod
    def get_category_urls(self) -> list[str]:
        """Return list of category URLs to scrape."""
        ...

    def scrape_all(self) -> list[ScrapedProduct]:
        """Scrape all categories and return all products."""
        all_products = []
        categories = self.get_category_urls()

        for i, url in enumerate(categories):
            logger.info(
                "[%s] Scraping category %d/%d: %s",
                self.store_name,
                i + 1,
                len(categories),
                url,
            )
            try:
                products = self.scrape_category(url)
                all_products.extend(products)
            except Exception:
                logger.exception("Failed to scrape %s", url)

            # Be respectful — wait between requests
            time.sleep(2)

        logger.info(
            "[%s] Scraped %d products total", self.store_name, len(all_products)
        )
        return all_products

    def close(self):
        self.client.close()
