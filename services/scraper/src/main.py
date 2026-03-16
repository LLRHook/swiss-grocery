import logging

from apscheduler.schedulers.blocking import BlockingScheduler

from src.db import get_connection, upsert_products, upsert_store_prices
from src.scrapers.migros import MigrosScraper
from src.scrapers.coop import CoopScraper
from src.scrapers.aldi import AldiScraper
from src.scrapers.lidl import LidlScraper
from src.scrapers.denner import DennerScraper

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(levelname)s] %(name)s: %(message)s",
)
logger = logging.getLogger(__name__)

SCRAPERS = [
    MigrosScraper,
    CoopScraper,
    AldiScraper,
    LidlScraper,
    DennerScraper,
]


def run_scrape():
    """Run all scrapers and persist results."""
    conn = get_connection()

    for ScraperClass in SCRAPERS:
        scraper = ScraperClass()
        try:
            products = scraper.scrape_all()
            if not products:
                continue

            # Convert to dicts for DB upsert
            product_dicts = [
                {
                    "name": p.name,
                    "brand": p.brand,
                    "ean": p.ean,
                    "unit": p.unit,
                    "unit_quantity": p.unit_quantity,
                    "image_url": p.image_url,
                }
                for p in products
            ]
            upsert_products(conn, product_dicts)

            # Upsert prices
            price_dicts = [
                {
                    "product_id": pd["id"],
                    "external_id": p.external_id,
                    "price": p.price,
                    "product_url": p.product_url,
                }
                for p, pd in zip(products, product_dicts)
            ]
            upsert_store_prices(conn, scraper.store_slug, price_dicts)

            logger.info(
                "Persisted %d products for %s",
                len(products),
                scraper.store_name,
            )
        except Exception:
            logger.exception("Scraper %s failed", scraper.store_name)
        finally:
            scraper.close()

    conn.close()


if __name__ == "__main__":
    logger.info("Running initial scrape...")
    run_scrape()

    logger.info("Starting scheduler — scraping every 6 hours")
    scheduler = BlockingScheduler()
    scheduler.add_job(run_scrape, "interval", hours=6)
    scheduler.start()
